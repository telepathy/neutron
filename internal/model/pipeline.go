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

type RunnerConfig struct {
	CodebaseToken string // codebase access token
	CodebaseUrl   string // codebase API base URL
	ProjectId     string
	CommitSha     string
	ReportSha     string // GitLab: commit SHA for status reporting; Codeup: unused
	Trigger       string
	JobName       string
	GitRepoUrl    string
	GitPrivateKey string
	PipelineUrl   string
	TargetBranch  string // MR target branch, GitLab only
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
