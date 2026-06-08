package main

import (
	"log"
	"neutron/internal/model"
	"neutron/internal/service"
	"os"
)

func main() {
	runnerConfig := getConfig()
	skipTLS := os.Getenv("SKIP_TLS_VERIFY") == "true"
	reporter := NewGitlabReporter(runnerConfig, skipTLS)
	runner := service.NewRunner("/repo", runnerConfig.Trigger, runnerConfig.JobName, reporter)
	runner.Run()
}

func getConfig() model.RunnerConfig {
	codebaseUrl := os.Getenv("CODEBASE_URL")
	if codebaseUrl == "" {
		log.Fatalln("CODEBASE_URL is not set. Pipeline exit now.")
	}
	codebaseToken := os.Getenv("CODEBASE_TOKEN")
	if codebaseToken == "" {
		log.Fatalln("CODEBASE_TOKEN is not set. Pipeline exit now.")
	}
	projectId := os.Getenv("PROJECT_ID")
	if projectId == "" {
		log.Fatalln("PROJECT_ID is not set. Pipeline exit now.")
	}
	commitSha := os.Getenv("COMMIT_SHA")
	if commitSha == "" {
		log.Fatalln("COMMIT_SHA is not set. Pipeline exit now.")
	}
	reportSha := os.Getenv("REPORT_SHA")
	if reportSha == "" {
		log.Fatalln("REPORT_SHA is not set. Pipeline exit now.")
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
	return model.RunnerConfig{
		CodebaseToken: codebaseToken,
		CodebaseUrl:   codebaseUrl,
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
