package main

type Config struct {
	PullRequest struct {
		MaxAllowedCommits *int `yaml:"maxAllowedCommits"`
	} `yaml:"pullRequest"`

	Commit struct {
		MaxSubjectLength      *int     `yaml:"maxSubjectLength"`
		MinSubjectLength      *int     `yaml:"minSubjectLength"`
		SubjectMustMatchRegex []string `yaml:"subjectMustMatchRegex"`
		MaxBodyMessageLength *int `yaml:"maxBodyMessageLength"`
	} `yaml:"commit"`
}
