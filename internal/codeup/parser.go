package codeup

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"neutron/internal/parser"
	"strings"
	"time"
)

type WebhookRequest struct {
	WebhookType string     `json:"object_kind"`
	CodeSha     string     `json:"checkout_sha"`
	Ref         string     `json:"ref"`
	Project     Project    `json:"project"`
	ProjectId   int        `json:"project_id"`
	Repository  Repository `json:"repository"`
	Attributes  Attributes `json:"object_attributes"`
}

type Project struct {
	Id int `json:"id"`
}

type Repository struct {
	GitHttpUrl string `json:"git_http_url"`
	GitSshUrl  string `json:"git_ssh_url"`
}

type Attributes struct {
	Iid          int        `json:"local_id"`
	ProjectId    int        `json:"project_id"`
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

// extractOrgId extracts the organization ID from a Codeup repository URL.
// e.g. "http://codeup.devops.csdc.com/codeup/f6e73c53-.../SZ/repo.git" → "f6e73c53-..."
func extractOrgId(gitHttpUrl string) string {
	idx := strings.Index(gitHttpUrl, "/codeup/")
	if idx < 0 {
		return ""
	}
	rest := gitHttpUrl[idx+len("/codeup/"):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

func NewCodeupParser(requestBody io.ReadCloser, codeupHost string, token string, skipTLSVerify bool) (*Parser, error) {
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
	if ref == "" {
		return nil, fmt.Errorf("missing commit SHA in webhook payload (type: %s)", request.WebhookType)
	}
	projectId := request.Project.Id
	if projectId == 0 {
		projectId = request.ProjectId
	}
	if projectId == 0 {
		projectId = request.Attributes.ProjectId
	}
	if projectId == 0 {
		return nil, fmt.Errorf("missing project ID in webhook payload")
	}

	// Extract orgId from repository git_http_url
	// e.g. http://codeup.devops.csdc.com/codeup/f6e73c53-f6ad-447a-b1c6-083e28f9b814/SZ/.../repo.git
	orgId := extractOrgId(request.Repository.GitHttpUrl)
	if orgId == "" {
		return nil, fmt.Errorf("cannot extract organization ID from repository URL: %s", request.Repository.GitHttpUrl)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify},
		},
	}
	return &Parser{
		Base: parser.Base{
			AccessApiPath:  fmt.Sprintf("%s/oapi/v1/codeup/organizations/%s/repositories/%d/files/neutron.yaml", codeupHost, orgId, projectId),
			AccessToken:    token,
			AuthHeaderName: "x-yunxiao-token",
			Client:         client,
			CodeSha:        ref,
			ReportSha:      reportSha,
			TargetBranch:   targetBranch,
			Trigger:        trigger,
		},
		Request: request,
	}, nil
}
