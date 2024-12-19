package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"neutron/internal/model"
	"neutron/internal/service"
	"os"
	"path"
)

type RunnerConfig struct {
	GitlabToken string
	GitlabUrl   string
	ProjectId   string
	CommitSha   string
	JobName     string
	Trigger     model.TriggerType
}

func main() {
	runnerConfig := getConfig()
	reporter := NewGitlabReporter(runnerConfig)
	err := downloadProject(runnerConfig, "/pipeline")
	if err != nil {
		log.Fatal(err)
	}
	entries, err := os.ReadDir("/pipeline")
	if err != nil {
		log.Fatal(err)
	}
	var workingDir string
	for _, entry := range entries {
		if entry.IsDir() {
			if workingDir != "" {
				log.Fatal("Multi workdir.")
			}
			workingDir = entry.Name()
		}
	}
	if workingDir == "" {
		log.Fatal("No working directory.")
	}

	runner := service.NewRunner(path.Join("/pipeline", workingDir), runnerConfig.Trigger, runnerConfig.JobName, reporter)
	runner.Run()
}

func downloadProject(c RunnerConfig, destDir string) error {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	baseUrl := fmt.Sprintf("%s/api/v4/projects/%s/repository/archive", c.GitlabUrl, c.ProjectId)
	u, _ := url.Parse(baseUrl)
	params := url.Values{}
	params.Add("sha", c.CommitSha)
	u.RawQuery = params.Encode()
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Add("PRIVATE-TOKEN", c.GitlabToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	return service.Extract(resp.Body, destDir)
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
	return RunnerConfig{
		GitlabToken: gitlabToken,
		GitlabUrl:   gitlabUrl,
		ProjectId:   projectId,
		CommitSha:   commitSha,
		JobName:     jobName,
		Trigger:     trigger,
	}
}
