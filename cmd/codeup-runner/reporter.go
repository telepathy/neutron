package main

import (
	"log"
	"neutron/internal/model"
)

// NoOpReporter logs pipeline status as TODO since Codeup has no pipeline status API.
type NoOpReporter struct{}

func NewNoOpReporter() *NoOpReporter {
	return &NoOpReporter{}
}

func (r *NoOpReporter) Report(jobName string, stepName string, status model.StepResult, description string) {
	log.Printf("[TODO] Pipeline status: %s/%s status=%s desc=%s", jobName, stepName, status, description)
}
