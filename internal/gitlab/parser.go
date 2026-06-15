package gitlab

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"neutron/internal/parser"
	"time"
)

type WebhookRequest struct {
	WebhookType string     `json:"object_kind"`
	CodeSha     string     `json:"checkout_sha"`
	Ref         string     `json:"ref"`
	Project     Project    `json:"project"`
	Attributes  Attributes `json:"object_attributes"`
}

type Project struct {
	Id        int    `json:"id"`
	RepoUrl   string `json:"http_url"`
	GitSshUrl string `json:"git_ssh_url"`
}

type Attributes struct {
	Iid          int        `json:"iid"`
	Action       string     `json:"action"`
	TargetBranch string     `json:"target_branch"`
	LastCommit   LastCommit `json:"last_commit"`
}

type LastCommit struct {
	Id string `json:"id"`
}

type Parser struct {
	parser.Base
	Request WebhookRequest
}

func NewGitLabParser(requestBody io.ReadCloser, gitlabHost string, token string, skipTLSVerify bool) (*Parser, error) {
	body, err := parser.ReadBody(requestBody)
	if err != nil {
		return nil, err
	}
	var request WebhookRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("parsing webhook body: %w", err)
	}

	trigger, ref, reportSha, targetBranch, err := parser.DetectTrigger(
		request.WebhookType, request.CodeSha, request.Attributes.LastCommit.Id, request.Attributes.TargetBranch,
	)
	if err != nil {
		return nil, err
	}

	// Skip MR events for merge/close actions (only trigger on open/reopen/update)
	if trigger == "MR" {
		action := request.Attributes.Action
		if action == "merge" || action == "close" {
			return nil, fmt.Errorf("skipping MR action: %s", action)
		}
	}

	if ref == "" {
		return nil, fmt.Errorf("missing commit SHA in webhook payload (type: %s)", request.WebhookType)
	}

	// Construct API path from SSH URL instead of project ID
	if request.Project.GitSshUrl == "" {
		return nil, fmt.Errorf("missing git_ssh_url in webhook payload")
	}
	projectPath := parser.ExtractGitLabProjectPath(request.Project.GitSshUrl)
	if projectPath == "" {
		return nil, fmt.Errorf("cannot extract project path from SSH URL: %s", request.Project.GitSshUrl)
	}
	encodedPath := url.PathEscape(projectPath)

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify},
		},
	}
	return &Parser{
		Base: parser.Base{
			AccessApiPath: fmt.Sprintf("%s/api/v4/projects/%s/repository/files/neutron.yaml", gitlabHost, encodedPath),
			AccessToken:   token,
			Client:        client,
			CodeSha:       ref,
			ReportSha:     reportSha,
			TargetBranch:  targetBranch,
			Trigger:       trigger,
		},
		Request: request,
	}, nil
}
