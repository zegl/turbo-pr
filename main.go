package main

import (
	"context"
	"log"
	"fmt"
	"strings"
	"net/http"
	"regexp"
	"flag"
	"strconv"
	"errors"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"github.com/bradleyfalzon/ghinstallation"
	"gopkg.in/yaml.v2"
)

var flagHttpPort *int
var flagGitHubAppKey *string
var flagGitHubIntegrationID *int
var flagGoogleCloudProjectID *string
var flagGoogleCloudCredentialsFile *string

func main() {
	flagHttpPort = flag.Int("port", 80, "HTTP port")
	flagGitHubAppKey = flag.String("github-key", "", "GitHub App Private Key")
	flagGitHubIntegrationID = flag.Int("github-id", 5073, "GitHub App ID")
	flagGoogleCloudProjectID = flag.String("gc-id", "", "Google Cloud Project ID")
	flagGoogleCloudCredentialsFile = flag.String("gc-credentials", "", "Google Cloud Credentials")

	flag.Parse()

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", *flagGoogleCloudCredentialsFile)

	webserver()
}

func webserver() {
	r := gin.Default()

	// Webhook handler
	r.POST("/gh-webhook", webhook)

	// Index
	r.GET("/", func(c *gin.Context) {
		logger("index-visit", "index-visit")
		c.Redirect(http.StatusFound, "https://github.com/zegl/turbo-pr")
	})

	err := r.Run(":" + strconv.Itoa(*flagHttpPort))
	if err != nil {
		panic(err)
	}
}

// getGitHubClient creates a GitHub client for the installationID
func getGitHubClient(installationID int) *github.Client {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, *flagGitHubIntegrationID, installationID, *flagGitHubAppKey)
	if err != nil {
		panic(err)
	}

	return github.NewClient(&http.Client{Transport: itr})
}

func webhook(c *gin.Context) {
	payload, err := github.ValidatePayload(c.Request, []byte("hello"))
	if err != nil {
		panic(err)
	}

	// Log incoming event to Stackdriver
	logger(c.GetHeader("X-Github-Event"), string(payload))

	event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
	if err != nil {
		panic(err)
	}

	switch event := event.(type) {
	case *github.PullRequestEvent:
		webhookPullRequest(event)
	}
}

func webhookPullRequest(ev *github.PullRequestEvent) {
	checkActions := map[string]struct{}{
		"opened":      {},
		"synchronize": {},
	}

	allStatuses := map[string]bool{
		"commit-count":    true,
		"commit-title":    true,
		"subject-length":  true,
		"body-row-length": true,
		"setup":           true,
	}

	// Action that we don't care about
	if _, ok := checkActions[*ev.Action]; !ok {
		return
	}

	gh := getGitHubClient(*ev.Installation.ID)

	config, err := webhookGetConfigFile(gh, ev)
	if err != nil {
		log.Println(err)
		// Send status to GH
		webhookSetStatus(gh, ev, "failure", "turbo-pr.yaml not found", "setup")
		return
	}

	// Amount of commits per PR
	if config.PullRequest.MaxAllowedCommits != nil {
		// More than one commit? Denied!
		if *ev.PullRequest.Commits > *config.PullRequest.MaxAllowedCommits {
			// Send status to GH
			webhookSetStatus(gh, ev, "failure",
				fmt.Sprintf("PR contains more than %d commit", *config.PullRequest.MaxAllowedCommits),
				"commit-count")

			// Mark as failed locally
			allStatuses["commit-count"] = false
		}
	}

	// Get Commit from GitHub
	commit, err := webhookGetCommit(gh, ev)

	if err != nil {
		panic(err)
	}

	// Commit subject length
	if config.Commit.MaxSubjectLength != nil && config.Commit.MinSubjectLength != nil {
		maxLen := *config.Commit.MaxSubjectLength
		minLen := *config.Commit.MinSubjectLength

		if !commitMessageSubjectIsValid(*commit.Commit.Message, maxLen, minLen) {
			// Send status to GH
			webhookSetStatus(gh, ev, "failure",
				fmt.Sprintf("Invalid subject message length: Min:%d, Max:%d", minLen, maxLen),
				"subject-length")

			// Mark as failed locally
			allStatuses["subject-length"] = false
		}
	}

	// Commit body row length
	if config.Commit.MaxBodyMessageLength != nil {
		if !commitMessageBodyIsValid(*commit.Commit.Message, *config.Commit.MaxBodyMessageLength) {
			// Send status to GH
			webhookSetStatus(gh, ev, "failure",
				fmt.Sprintf("Invalid body message length: Max %d chars per row", *config.Commit.MaxBodyMessageLength),
				"body-row-length")

			// Mark as failed locally
			allStatuses["body-row-length"] = false
		}
	}

	// Commit message regex
	if len(config.Commit.SubjectMustMatchRegex) > 0 {
		success := false

		for _, re := range config.Commit.SubjectMustMatchRegex {
			if commitMessageMatchesRegex(*commit.Commit.Message, re) {
				success = true
			}
		}

		if !success {
			// Send status to GH
			webhookSetStatus(gh, ev, "failure",
				"Commit message does not match any valid format",
				"commit-title")

			// Mark as failed locally
			allStatuses["commit-title"] = false
		}
	}

	// Mark the checks that did not fail as successful
	for statusID, statusSuccess := range allStatuses {
		if statusSuccess {
			webhookSetStatus(gh, ev, "success", "OK!", statusID)
		}
	}
}

func webhookSetStatus(gh *github.Client, ev *github.PullRequestEvent, state string, description string, statusContext string) {
	gh.Repositories.CreateStatus(
		context.Background(),
		*ev.Repo.Owner.Login,
		*ev.Repo.Name,
		*ev.PullRequest.Head.SHA,
		&github.RepoStatus{
			State:       github.String(state),
			Description: github.String(description),
			Context:     github.String(statusContext),
		},
	)
}

func webhookGetCommit(gh *github.Client, ev *github.PullRequestEvent) (*github.RepositoryCommit, error) {
	commit, _, err := gh.Repositories.GetCommit(
		context.Background(),
		*ev.Repo.Owner.Login,
		*ev.Repo.Name,
		*ev.PullRequest.Head.SHA,
	)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	return commit, nil
}

// webhookGetConfigFile loads a *Config based from the turbo-pr.yaml file in the repo
func webhookGetConfigFile(gh *github.Client, ev *github.PullRequestEvent) (*Config, error) {
	file, _, _, err := gh.Repositories.GetContents(
		context.Background(),
		*ev.Repo.Owner.Login,
		*ev.Repo.Name,
		"turbo-pr.yaml",
		&github.RepositoryContentGetOptions{
			Ref: *ev.PullRequest.Head.SHA,
		},
	)
	if err != nil {
		return nil, err
	}

	if file.GetEncoding() != "base64" {
		return nil, errors.New("Encoding was not base64: " + file.GetEncoding())
	}

	fileContent, err := file.GetContent()
	if err != nil {
		return nil, err
	}

	var conf Config
	err = yaml.Unmarshal([]byte(fileContent), &conf)
	if err != nil {
		return nil, err
	}

	return &conf, nil
}

func commitMessageSubjectIsValid(message string, subjectMaxLen, subjectMinLen int) bool {
	messageRows := strings.Split(message, "\n")
	messageSubject := messageRows[0]

	// Commit subject length
	if len(messageSubject) > subjectMaxLen || len(messageSubject) < subjectMinLen {
		return false
	}

	return true
}

// commitMessageBodyIsValid tests the length of the rows in the body is of an allowed length
// message should be the full commit message, including the first row
func commitMessageBodyIsValid(message string, bodyMaxRowLen int) bool {
	messageRows := strings.Split(message, "\n")

	// Body lengths
	if len(messageRows) > 1 {
		for _, row := range messageRows[1:] {
			if len(row) > bodyMaxRowLen {
				return false
			}
		}
	}

	return true
}

func commitMessageMatchesRegex(message, regex string) bool {
	message = strings.Split(message, "\n")[0]

	r, err := regexp.Compile(regex)
	if err != nil {
		return false
	}

	return r.MatchString(message)
}
