package model

type Pipeline struct {
	Jobs map[string]Job `yaml:"jobs"`
}

type Job struct {
	Image     string     `yaml:"image"`
	Trigger   []string   `yaml:"trigger"`
	Steps     []Step     `yaml:"steps"`
	Resources *Resources `yaml:"resources,omitempty"`
}

type Step struct {
	StepName string `yaml:"name"`
	Command  string `yaml:"cmd"`
}

type Resources struct {
	Limits   ResourceSpec `yaml:"limits,omitempty"`
	Requests ResourceSpec `yaml:"requests,omitempty"`
}

type ResourceSpec struct {
	Cpu    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

type RunnerConfig struct {
	CodebaseToken      string // codebase access token
	CodebaseUrl        string // codebase API base URL
	ProjectId          string
	CommitSha          string
	ReportSha          string // GitLab: commit SHA for status reporting; Codeup: unused
	Trigger            string
	JobName            string
	GitRepoUrl         string
	GitPrivateKey      string
	PipelineUrl        string
	TargetBranch       string // MR target branch, GitLab only
	SkipTriggerCheck   bool   // skip trigger type validation (for API-triggered jobs)
	SkipPlatformReport bool   // skip reporting commit status to platform (for API-triggered jobs)
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
