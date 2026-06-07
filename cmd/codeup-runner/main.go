package main

import (
	"log"
	"neutron/internal/model"
	"neutron/internal/service"
	"os"
)

func main() {
	runnerConfig := getConfig()
	reporter := NewNoOpReporter()
	runner := service.NewRunner("/repo", runnerConfig.Trigger, runnerConfig.JobName, reporter)
	runner.Run()
}

func getConfig() model.RunnerConfig {
	commitSha := os.Getenv("COMMIT_SHA")
	if commitSha == "" {
		log.Fatalln("COMMIT_SHA is not set. Pipeline exit now.")
	}
	trigger := os.Getenv("TRIGGER")
	if trigger == "" {
		log.Fatalln("TRIGGER is not set. Pipeline exit now.")
	}
	jobName := os.Getenv("JOB_NAME")
	if jobName == "" {
		log.Fatalln("JOB_NAME is not set. Pipeline exit now.")
	}
	gitPrivateKey := os.Getenv("GIT_PRIVATE_KEY")
	if gitPrivateKey == "" {
		log.Fatalln("GIT_PRIVATE_KEY is not set. Pipeline exit now.")
	}
	gitRepoUrl := os.Getenv("GIT_REPO_URL")
	if gitRepoUrl == "" {
		log.Fatalln("GIT_REPO_URL is not set. Pipeline exit now.")
	}
	return model.RunnerConfig{
		CommitSha:     commitSha,
		JobName:       jobName,
		Trigger:       trigger,
		GitPrivateKey: gitPrivateKey,
		GitRepoUrl:    gitRepoUrl,
	}
}
