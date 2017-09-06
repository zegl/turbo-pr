package main

import (
	"context"
	"log"

	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var gh *github.Client
var config *Config

func main() {
	config = getConfig("conf.yaml")

	//log.Printf("%+v", config)
	//return

	setupGitHubClient()
	go webserver()
	select {}
}

func setupGitHubClient() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: "4b9fbcf38c49d42ef49fa4f5d11e9bb13c813f81"},
	)
	tc := oauth2.NewClient(ctx, ts)

	gh = github.NewClient(tc)
}

func webserver() {
	r := gin.Default()
	r.POST("/gh-webhook", webhook)
	r.Run("127.0.0.1:8080")
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
		"commit-count":          true,
		// "commit-title":          true,
		"subject-length":        true,
		// "commit-message-length": true,
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
	commitMessage := strings.Split(*commit.Commit.Message, "\n")
	log.Printf("%+v", commitMessage)

	// Commit subject length
	if config.Commit.MaxSubjectLength != nil || config.Commit.MinSubjectLength != nil {
		maxLen := 100000
		minLen := 0

		if config.Commit.MaxSubjectLength != nil {
			maxLen = *config.Commit.MaxSubjectLength
		}
		if config.Commit.MinSubjectLength != nil {
			minLen = *config.Commit.MinSubjectLength
		}

		commitLen := len(commitMessage[0])

		success := true

		if commitLen > maxLen || commitLen < minLen {
			success = false
		}

		if !success {
			// Send status to GH
			webhookSetStatus(ev, "failure",
				fmt.Sprintf("Commit subject message length: %d (Min:%d, Max:%d)",
					commitLen, minLen, maxLen,
				),
				"subject-length")

			// Mark as failed locally
			allStatuses["subject-length"] = false
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
