package parser

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
	Iid int `json:"iid"`
}

type FileResponse struct {
	Content string `json:"content"`
}

type GitLabParser struct {
	accessApiPath string
	accessToken   string
	Request       WebhookRequest
}

func NewGitLabParser(requestBody io.ReadCloser, gitlabHost string, token string) *GitLabParser {
	var request WebhookRequest
	body, _ := io.ReadAll(requestBody)
	defer requestBody.Close()
	_ = json.Unmarshal(body, &request)

	return &GitLabParser{
		accessApiPath: fmt.Sprintf("%s/api/v4/projects/%d/repository/files/neutron.yaml", gitlabHost, request.Project.Id),
		accessToken:   token,
		Request:       request,
	}
}

func (g *GitLabParser) Parse() (model.Pipeline, error) {
	var ref string
	switch g.Request.WebhookType {
	case "merge_request":
		ref = fmt.Sprintf("refs/merge-requests/%d/merge", g.Request.Attributes.Iid)
	case "tag_push":
		ref = g.Request.Ref
	case "push":
		ref = g.Request.CodeSha
	default:
		ref = g.Request.CodeSha
	}
	req, err := http.NewRequest("GET", g.accessApiPath, nil)
	if err != nil {
		return model.Pipeline{}, err
	}
	query := req.URL.Query()
	query.Add("ref", ref)
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
