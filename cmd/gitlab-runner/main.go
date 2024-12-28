package main

import (
	"log"
	"neutron/internal/model"
	"neutron/internal/service"
	"os"
	"strings"
)

type RunnerConfig struct {
	GitlabToken string
	GitlabUrl   string
	ProjectId   string
	CommitSha   string
	JobName     string
	GitRepoUrl  string
	GitUsername string
	GitPassword string
	Trigger     model.TriggerType
}

func main() {
	runnerConfig := getConfig()
	reporter := NewGitlabReporter(runnerConfig)
	err := downloadProject(runnerConfig, "/repo")
	if err != nil {
		log.Fatal(err)
	}

	runner := service.NewRunner("/repo", runnerConfig.Trigger, runnerConfig.JobName, reporter)
	runner.Run()
}

func downloadProject(c RunnerConfig, destDir string) error {
	var err error
	if strings.Contains(c.CommitSha, "refs/") {
		err = service.CheckoutRef(c.GitRepoUrl, c.CommitSha, c.GitUsername, c.GitPassword, destDir)
	} else {
		err = service.CheckoutSha(c.GitRepoUrl, c.CommitSha, c.GitUsername, c.GitPassword, destDir)
	}
	return err
}

func getConfig() RunnerConfig {
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
	trigger := model.TriggerType(os.Getenv("TRIGGER"))
	if trigger == "" {
		log.Fatalln("GITLAB_TRIGGER is not set. Pipeline exit now.")
	}
	jobName := os.Getenv("JOB_NAME")
	if jobName == "" {
		log.Fatalln("JOB_NAME is not set. Pipeline exit now.")
	}
	gitUsername := os.Getenv("GIT_USERNAME")
	if gitUsername == "" {
		log.Fatalln("GIT_USERNAME is not set. Pipeline exit now.")
	}
	gitPassword := os.Getenv("GIT_PASSWORD")
	if gitPassword == "" {
		log.Fatalln("GIT_PASSWORD is not set. Pipeline exit now.")
	}
	gitRepoUrl := os.Getenv("GIT_REPO_URL")
	if gitRepoUrl == "" {
		log.Fatalln("GIT_REPO_URL is not set. Pipeline exit now.")
	}
	return RunnerConfig{
		GitlabToken: gitlabToken,
		GitlabUrl:   gitlabUrl,
		ProjectId:   projectId,
		CommitSha:   commitSha,
		JobName:     jobName,
		Trigger:     trigger,
		GitUsername: gitUsername,
		GitPassword: gitPassword,
		GitRepoUrl:  gitRepoUrl,
	}
}
