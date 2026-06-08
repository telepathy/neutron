package codeup

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"neutron/internal/parser"
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
		return nil, fmt.Errorf("missing project ID in webhook payload")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify},
		},
	}
	return &Parser{
		Base: parser.Base{
			AccessApiPath: fmt.Sprintf("%s/api/v4/projects/%d/repository/files/neutron.yaml", codeupHost, projectId),
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
