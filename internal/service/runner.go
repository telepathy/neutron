package service

import (
	"fmt"
	"github.com/kballard/go-shellquote"
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

func NewRunner(workingDir string, triggerType string, jobName string, reporter model.Reporter) *Runner {
	data, err := os.ReadFile(path.Join(workingDir, "neutron.yaml"))
	if err != nil {
		log.Fatalf(err.Error())
	}
	var pipeline model.Pipeline
	err = yaml.Unmarshal(data, &pipeline)
	if err != nil {
		log.Fatalf(err.Error())
	}
	if _, ok := pipeline.Jobs[jobName]; !ok {
		log.Fatalf("pipeline job %s not found", jobName)
	}
	flag := false
	for _, t := range pipeline.Jobs[jobName].Trigger {
		if t == triggerType {
			flag = true
			break
		}
	}
	if !flag {
		reporter.Report(jobName, "", model.Success, fmt.Sprintf("Current job skipped in %s.", triggerType))
		os.Exit(0)
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
		r.Reporter.Report(r.JobName, step.StepName, model.Running, "pipeline started.")
		parts, err := shellquote.Split(step.Command)
		if err != nil {
			r.Reporter.Report(r.JobName, step.StepName, model.Fail, "wrong command format.")
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = r.WorkingDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// failed current step and all after steps.
			for i, step := range r.Steps {
				if i >= runStepIndex {
					r.Reporter.Report(r.JobName, step.StepName, model.Fail, "pipeline failed.")
				}
			}
			os.Exit(1)
		} else {
			r.Reporter.Report(r.JobName, step.StepName, model.Success, "pipeline finished.")
		}
	}
}
