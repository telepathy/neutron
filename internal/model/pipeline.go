package model

type Pipeline struct {
	Jobs map[string]Job `yaml:"jobs"`
}

type Job struct {
	Image   string        `yaml:"image"`
	Trigger []TriggerType `yaml:"trigger"`
	Steps   []Step        `yaml:"steps"`
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
