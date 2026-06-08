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

type NeutronReporter struct {
	apiUrl      string
	jobName     string
	triggerType string
	webhookType string
	client      *http.Client
}

func NewNeutronReporter(apiUrl string, jobName string, triggerType string, webhookType string) *NeutronReporter {
	return &NeutronReporter{
		apiUrl:      apiUrl,
		jobName:     jobName,
		triggerType: triggerType,
		webhookType: webhookType,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (r *NeutronReporter) Report(jobName string, stepName string, status model.StepResult, description string) {
	payload := map[string]interface{}{
		"webhook_type": r.webhookType,
		"trigger_type": r.triggerType,
		"active":       0,
		"succeeded":    0,
		"failed":       0,
	}

	switch status {
	case model.Running, model.Pending:
		payload["active"] = 1
	case model.Success:
		payload["succeeded"] = 1
	case model.Fail:
		payload["failed"] = 1
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal status: %v", err)
		return
	}

	url := fmt.Sprintf("%s/api/report/%s", r.apiUrl, r.jobName)
	resp, err := r.client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Failed to report to Neutron API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Neutron API returned status %d", resp.StatusCode)
	}
}
