package gitlab

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"neutron/internal/model"
)

type WebhookRequest struct {
	WebhookType string     `json:"object_kind"`
	CodeSha     string     `json:"checkout_sha"`
	Ref         string     `json:"ref"`
	Project     Project    `json:"project"`
	Attributes  Attributes `json:"object_attributes"`
}

type Project struct {
	Id      int    `json:"id"`
	RepoUrl string `json:"http_url"`
}

type Attributes struct {
	Iid        int        `json:"iid"`
	LastCommit LastCommit `json:"last_commit"`
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
	CodeSha       string
	ReportSha     string
	Request       WebhookRequest
	Trigger       string
}

func NewGitLabParser(requestBody io.ReadCloser, gitlabHost string, token string) *Parser {
	var request WebhookRequest
	body, _ := io.ReadAll(requestBody)
	defer requestBody.Close()
	_ = json.Unmarshal(body, &request)
	var ref string
	var trigger string
	var reportSha string
	switch request.WebhookType {
	case "merge_request":
		ref = fmt.Sprintf("refs/merge-requests/%d/merge", request.Attributes.Iid)
		reportSha = request.Attributes.LastCommit.Id
		trigger = "MR"
	case "tag_push":
		ref = request.Ref
		reportSha = request.CodeSha
		trigger = "TAG"
	case "push":
		ref = request.CodeSha
		reportSha = request.CodeSha
		trigger = "PUSH"
	default:
		ref = request.CodeSha
		reportSha = request.CodeSha
		trigger = "PUSH"
	}
	return &Parser{
		accessApiPath: fmt.Sprintf("%s/api/v4/projects/%d/repository/files/neutron.yaml", gitlabHost, request.Project.Id),
		accessToken:   token,
		Request:       request,
		CodeSha:       ref,
		ReportSha:     reportSha,
		Trigger:       trigger,
	}
}

func (g *Parser) Parse() (model.Pipeline, error) {
	req, err := http.NewRequest("GET", g.accessApiPath, nil)
	if err != nil {
		return model.Pipeline{}, err
	}
	query := req.URL.Query()
	yamlRef := g.CodeSha
	if g.Trigger == "MR" {
		// in case of changing neutron.yaml, should get from last commit
		yamlRef = g.Request.Attributes.LastCommit.Id
	}
	query.Add("ref", yamlRef)
	req.URL.RawQuery = query.Encode()
	req.Header.Add("PRIVATE-TOKEN", g.accessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return model.Pipeline{}, err
	}
	defer res.Body.Close()
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
