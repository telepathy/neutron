package main

import (
	"neutron/internal/model"
)

type CompositeReporter struct {
	reporters []model.Reporter
}

func NewCompositeReporter(reporters ...model.Reporter) *CompositeReporter {
	return &CompositeReporter{
		reporters: reporters,
	}
}

func (r *CompositeReporter) Report(jobName string, stepName string, status model.StepResult, description string) {
	for _, reporter := range r.reporters {
		reporter.Report(jobName, stepName, status, description)
	}
}
