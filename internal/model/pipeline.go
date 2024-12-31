package model

import (
	batchv1 "k8s.io/api/batch/v1"
)

type Pipeline struct {
	Jobs map[string]Job `yaml:"jobs"`
}

type Job struct {
	Image   string   `yaml:"image"`
	Trigger []string `yaml:"trigger"`
	Steps   []Step   `yaml:"steps"`
}

type Step struct {
	StepName string `yaml:"name"`
	Command  string `yaml:"cmd"`
}

type StepResult string

const (
	Pending StepResult = "Pending"
	Running StepResult = "Running"
	Fail    StepResult = "Fail"
	Success StepResult = "Success"
)

type Reporter interface {
	Report(jobName string, stepName string, status StepResult, description string)
}

type PipelineParser interface {
	Parse() (Pipeline, error)
}

type JobCreator interface {
	CreateJob(jobName string) *batchv1.Job
}
