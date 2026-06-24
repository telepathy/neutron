package service

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"neutron/internal/model"
	"os"
	"os/exec"
	"path"
)

type Runner struct {
	WorkingDir string
	JobName    string
	Trigger    string
	Steps      []model.Step
	Reporter   model.Reporter
}

func NewRunner(workingDir string, triggerType string, jobName string, reporter model.Reporter, skipTriggerCheck ...bool) *Runner {
	data, err := os.ReadFile(path.Join(workingDir, "neutron.yaml"))
	if err != nil {
		log.Fatal(err)
	}
	var pipeline model.Pipeline
	err = yaml.Unmarshal(data, &pipeline)
	if err != nil {
		log.Fatal(err)
	}
	if _, ok := pipeline.Jobs[jobName]; !ok {
		log.Fatalf("pipeline job %s not found", jobName)
	}
	// Skip trigger check if requested (e.g. API-triggered jobs)
	skip := len(skipTriggerCheck) > 0 && skipTriggerCheck[0]
	if !skip {
		matched := false
		for _, t := range pipeline.Jobs[jobName].Trigger {
			if t == triggerType {
				matched = true
				break
			}
		}
		if !matched {
			reporter.Report(jobName, "", model.Success, fmt.Sprintf("Current job skipped in %s.", triggerType))
			os.Exit(0)
		}
	}
	return &Runner{
		WorkingDir: workingDir,
		Trigger:    triggerType,
		JobName:    jobName,
		Steps:      pipeline.Jobs[jobName].Steps,
		Reporter:   reporter,
	}
}

func (r *Runner) Run() {
	// create all step status
	for _, step := range r.Steps {
		r.Reporter.Report(r.JobName, step.StepName, model.Pending, "pipeline created.")
	}

	// run in seq
	for runStepIndex, step := range r.Steps {
		if step.Command == "" {
			r.Reporter.Report(r.JobName, step.StepName, model.Fail, "empty command.")
			r.failRemaining(runStepIndex)
			os.Exit(1)
		}
		r.Reporter.Report(r.JobName, step.StepName, model.Running, "pipeline started.")
		cmd := exec.Command("sh", "-c", step.Command)
		cmd.Dir = r.WorkingDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			errMsg := fmt.Sprintf("step failed: %v", err)
			r.Reporter.Report(r.JobName, step.StepName, model.Fail, errMsg)
			r.failRemaining(runStepIndex + 1)
			os.Exit(1)
		}
		r.Reporter.Report(r.JobName, step.StepName, model.Success, "pipeline finished.")
	}
}

func (r *Runner) failRemaining(fromIndex int) {
	for i := fromIndex; i < len(r.Steps); i++ {
		r.Reporter.Report(r.JobName, r.Steps[i].StepName, model.Fail, "pipeline failed.")
	}
}
