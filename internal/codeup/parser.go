package codeup

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"neutron/internal/model"
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

type FileResponse struct {
	Content string `json:"content"`
}

type Parser struct {
	accessApiPath string
	accessToken   string
	client        *http.Client
	CodeSha       string
	ReportSha     string
	TargetBranch  string
	Request       WebhookRequest
	Trigger       string
}

const maxWebhookBodySize = 1 << 20 // 1MB

func NewCodeupParser(requestBody io.ReadCloser, codeupHost string, token string, skipTLSVerify bool) (*Parser, error) {
	var request WebhookRequest
	body, err := io.ReadAll(io.LimitReader(requestBody, maxWebhookBodySize))
	defer requestBody.Close()
	if err != nil {
		return nil, fmt.Errorf("reading webhook body: %w", err)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("parsing webhook body: %w", err)
	}
	var ref string
	var trigger string
	var reportSha string
	var targetBranch string
	switch request.WebhookType {
	case "merge_request":
		ref = request.Attributes.LastCommit.Id
		reportSha = request.Attributes.LastCommit.Id
		targetBranch = request.Attributes.TargetBranch
		trigger = "MR"
	case "tag_push":
		ref = request.CodeSha
		reportSha = request.CodeSha
		trigger = "TAG"
	case "push":
		ref = request.CodeSha
		reportSha = request.CodeSha
		trigger = "PUSH"
	default:
		return nil, fmt.Errorf("unsupported webhook type: %s", request.WebhookType)
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
		accessApiPath: fmt.Sprintf("%s/api/v4/projects/%d/repository/files/neutron.yaml", codeupHost, projectId),
		accessToken:   token,
		client:        client,
		Request:       request,
		CodeSha:       ref,
		ReportSha:     reportSha,
		TargetBranch:  targetBranch,
		Trigger:       trigger,
	}, nil
}

func (g *Parser) Parse() (model.Pipeline, error) {
	req, err := http.NewRequest("GET", g.accessApiPath, nil)
	if err != nil {
		return model.Pipeline{}, err
	}
	query := req.URL.Query()
	query.Add("ref", g.CodeSha)
	req.URL.RawQuery = query.Encode()
	req.Header.Add("PRIVATE-TOKEN", g.accessToken)
	res, err := g.client.Do(req)
	if err != nil {
		return model.Pipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return model.Pipeline{}, fmt.Errorf("neutron.yaml not found in repository (ref: %s)", g.CodeSha)
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return model.Pipeline{}, fmt.Errorf("authentication failed when accessing Codeup API (status: %d)", res.StatusCode)
	}
	if res.StatusCode >= 400 {
		return model.Pipeline{}, fmt.Errorf("Codeup API returned error (status: %d)", res.StatusCode)
	}
	var fileResponse FileResponse
	err = json.NewDecoder(res.Body).Decode(&fileResponse)
	if err != nil {
		return model.Pipeline{}, err
	}
	neutronContent, err := base64.StdEncoding.DecodeString(fileResponse.Content)
	if err != nil {
		return model.Pipeline{}, err
	}
	var pipeline model.Pipeline
	err = yaml.Unmarshal(neutronContent, &pipeline)
	return pipeline, err
}
