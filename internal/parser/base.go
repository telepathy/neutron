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

const MaxBodySize = 1 << 20 // 1MB

type FileResponse struct {
	Content string `json:"content"`
}

type Base struct {
	AccessApiPath  string
	AccessToken    string
	AuthHeaderName string // e.g. "PRIVATE-TOKEN" (GitLab), "x-yunxiao-token" (Codeup)
	Client         *http.Client
	CodeSha        string
	ReportSha      string
	TargetBranch   string
	Trigger        string
}

func (b *Base) Parse() (model.Pipeline, error) {
	req, err := http.NewRequest("GET", b.AccessApiPath, nil)
	if err != nil {
		return model.Pipeline{}, err
	}
	query := req.URL.Query()
	query.Add("ref", b.CodeSha)
	req.URL.RawQuery = query.Encode()
	authHeader := b.AuthHeaderName
	if authHeader == "" {
		authHeader = "PRIVATE-TOKEN"
	}
	req.Header.Add(authHeader, b.AccessToken)
	res, err := b.Client.Do(req)
	if err != nil {
		return model.Pipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return model.Pipeline{}, fmt.Errorf("neutron.yaml not found in repository (ref: %s)", b.CodeSha)
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return model.Pipeline{}, fmt.Errorf("authentication failed when accessing API (status: %d)", res.StatusCode)
	}
	if res.StatusCode >= 400 {
		return model.Pipeline{}, fmt.Errorf("API returned error (status: %d)", res.StatusCode)
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

// ReadBody reads and closes the request body with size limit.
func ReadBody(body io.ReadCloser) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, MaxBodySize))
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading webhook body: %w", err)
	}
	return data, nil
}

// DetectTrigger returns trigger type, ref SHA, report SHA, target branch from common webhook fields.
func DetectTrigger(webhookType, codeSha, lastCommitId, targetBranch string) (trigger, ref, reportSha, tBranch string, err error) {
	switch webhookType {
	case "merge_request":
		return "MR", lastCommitId, lastCommitId, targetBranch, nil
	case "tag_push":
		return "TAG", codeSha, codeSha, "", nil
	case "push":
		return "PUSH", codeSha, codeSha, "", nil
	default:
		return "", "", "", "", fmt.Errorf("unsupported webhook type: %s", webhookType)
	}
}
