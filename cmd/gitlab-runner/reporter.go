package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"neutron/internal/model"
	"time"
)

type message struct {
	State       string `json:"state"`
	TargetUrl   string `json:"target_url"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

type GitlabReporter struct {
	client    *http.Client
	config    model.RunnerConfig
	url       string
	targetUrl string
}

func NewGitlabReporter(c model.RunnerConfig) *GitlabReporter {
	return &GitlabReporter{
		config:    c,
		client:    &http.Client{Timeout: 10 * time.Second},
		url:       fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", c.CodebaseUrl, c.ProjectId, c.ReportSha),
		targetUrl: c.PipelineUrl,
	}
}

func (r *GitlabReporter) Report(jobName string, stepName string, status model.StepResult, description string) {
	m := message{
		TargetUrl:   r.targetUrl,
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
	body, err := json.Marshal(m)
	if err != nil {
		log.Printf("Warning: failed to marshal status: %v", err)
		return
	}
	req, err := http.NewRequest("POST", r.url, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Warning: failed to create request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", r.config.CodebaseToken)
	resp, err := r.client.Do(req)
	if err != nil {
		log.Printf("Warning: failed to report pipeline status to gitlab: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("Warning: gitlab returned %s for %s/%s", resp.Status, jobName, stepName)
	} else {
		log.Printf("Pipeline status reported to gitlab: %s", resp.Status)
	}
}
