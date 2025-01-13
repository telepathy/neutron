package main

import (
	"log"
	"neutron/internal/gitlab"
	"neutron/internal/service"
	"os"
)

func main() {
	runnerConfig := getConfig()
	reporter := NewGitlabReporter(runnerConfig)
	runner := service.NewRunner("/repo", runnerConfig.Trigger, runnerConfig.JobName, reporter)
	runner.Run()
}

func getConfig() gitlab.RunnerConfig {
	gitlabUrl := os.Getenv("GITLAB_URL")
	if gitlabUrl == "" {
		log.Fatalln("GITLAB_URL is not set. Pipeline exit now.")
	}
	gitlabToken := os.Getenv("GITLAB_TOKEN")
	if gitlabToken == "" {
		log.Fatalln("GITLAB_TOKEN is not set. Pipeline exit now.")
	}
	projectId := os.Getenv("GITLAB_PROJECT_ID")
	if projectId == "" {
		log.Fatalln("GITLAB_URL is not set. Pipeline exit now.")
	}
	commitSha := os.Getenv("GITLAB_COMMIT_SHA")
	if commitSha == "" {
		log.Fatalln("GITLAB_COMMIT_SHA is not set. Pipeline exit now.")
	}
	reportSha := os.Getenv("GITLAB_REPORT_SHA")
	if reportSha == "" {
		log.Fatalln("GITLAB_REPORT_SHA is not set. Pipeline exit now.")
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
	pipelineUrl := os.Getenv("PIPELINE_URL")
	if pipelineUrl == "" {
		log.Fatalln("PIPELINE_URL is not set. Pipeline exit now.")
	}
	return gitlab.RunnerConfig{
		GitlabToken:   gitlabToken,
		GitlabUrl:     gitlabUrl,
		ProjectId:     projectId,
		CommitSha:     commitSha,
		ReportSha:     reportSha,
		JobName:       jobName,
		Trigger:       trigger,
		GitPrivateKey: gitPrivateKey,
		GitRepoUrl:    gitRepoUrl,
		PipelineUrl:   pipelineUrl,
	}
}
