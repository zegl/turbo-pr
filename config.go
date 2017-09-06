package main

import "io/ioutil"
import "gopkg.in/yaml.v2"


type Config struct {
	PullRequest struct {
		MaxAllowedCommits *int `yaml:"maxAllowedCommits"`
	} `yaml:"pullRequest"`

	Commit struct {
		MaxSubjectLength      *int     `yaml:"maxSubjectLength"`
		MinSubjectLength      *int     `yaml:"minSubjectLength"`
		SubjectMustMatchRegex []string `yaml:"subjectMustMatchRegex"`
	} `yaml:"commit"`
}

func getConfig(configFile string) *Config {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		panic(err)
	}

	var conf Config

	err = yaml.Unmarshal(configData, &conf)
	if err != nil {
		panic(err)
	}

	return &conf
}