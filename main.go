package main

import (
	"context"
	"log"

	"fmt"
	"strings"

	"net/http"
	"os"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var gh *github.Client
var config *Config

func main() {
	config = getConfig("conf.yaml")

	setupGitHubClient()
	go webserver()
	select {}
}

func setupGitHubClient() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	gh = github.NewClient(tc)
}

func webserver() {
	r := gin.Default()
	r.POST("/gh-webhook", webhook)
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "turbo-pr") })
	r.Run(":80")
}

func webhook(c *gin.Context) {
	payload, err := github.ValidatePayload(c.Request, []byte("hello"))
	if err != nil {
		panic(err)
	}

	event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
	if err != nil {
		panic(err)
	}

	switch event := event.(type) {
	case *github.PullRequestEvent:
		webhookPullRequest(event)
	}

	log.Printf("Unknown event: %s", c.GetHeader("X-GitHub-Event"))
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
	}

	// Action that we don't care about
	if _, ok := checkActions[*ev.Action]; !ok {
		return
	}

	// Amount of commits per PR
	if config.PullRequest.MaxAllowedCommits != nil {
		// More than one commit? Denied!
		if *ev.PullRequest.Commits > *config.PullRequest.MaxAllowedCommits {
			// Send status to GH
			webhookSetStatus(ev, "failure",
				fmt.Sprintf("PR contains more than %d commit", *config.PullRequest.MaxAllowedCommits),
				"commit-count")

			// Mark as failed locally
			allStatuses["commit-count"] = false
		}
	}

	// Get Commit from GitHub
	commit := webhookGetCommit(ev)

	// Commit subject length
	if config.Commit.MaxSubjectLength != nil && config.Commit.MinSubjectLength != nil {
		maxLen := *config.Commit.MaxSubjectLength
		minLen := *config.Commit.MinSubjectLength

		if !commitMessageSubjectIsValid(*commit.Commit.Message, maxLen, minLen) {
			// Send status to GH
			webhookSetStatus(ev, "failure",
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
			webhookSetStatus(ev, "failure",
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
			webhookSetStatus(ev, "failure",
				"Commit message does not match any valid format",
				"commit-title")

			// Mark as failed locally
			allStatuses["commit-title"] = false
		}

	}

	// Mark the checks that did not fail as successful
	for statusID, statusSuccess := range allStatuses {
		if statusSuccess {
			webhookSetStatus(ev, "success", "OK!", statusID)
		}
	}
}

func webhookSetStatus(ev *github.PullRequestEvent, state string, description string, statusContext string) {
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

func webhookGetCommit(ev *github.PullRequestEvent) *github.RepositoryCommit {
	commit, _, err := gh.Repositories.GetCommit(
		context.Background(),
		*ev.Repo.Owner.Login,
		*ev.Repo.Name,
		*ev.PullRequest.Head.SHA,
	)
	if err != nil {
		panic(err)
	}

	return commit
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
