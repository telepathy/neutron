package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"neutron/internal/model"
)

type message struct {
	State       string `json:"state"`
	TargetUrl   string `json:"target_url"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

type GitlabReporter struct {
	client  *http.Client
	gitBase RunnerConfig
	url     string
}

func NewGitlabReporter(c RunnerConfig) *GitlabReporter {
	return &GitlabReporter{
		gitBase: c,
		client:  &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}},
		url:     fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", c.GitlabUrl, c.ProjectId, c.CommitSha),
	}
}

func (r *GitlabReporter) Report(jobName string, stepName string, status model.StepResult, description string) {
	m := message{
		TargetUrl:   "http://localhost",
		Description: description,
		Context:     fmt.Sprintf("%s/%s", jobName, stepName),
	}
	switch status {
	case model.Pending:
		m.State = "pending"
	case model.Running:
		m.State = "running"
	case model.Fail:
		m.State = "failed"
	case model.Success:
		m.State = "success"
	default:
		m.State = "failed"
	}
	body, _ := json.Marshal(m)
	req, _ := http.NewRequest("POST", r.url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", r.gitBase.GitlabToken)
	_, err := r.client.Do(req)
	if err != nil {
		log.Fatalf("Error sending pipeline status to gitlab: %v", err)
	}
}
