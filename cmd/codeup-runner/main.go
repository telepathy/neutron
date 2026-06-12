package main

import (
	"neutron/internal/reporter"
	"neutron/internal/service"
	"os"
	"strings"
)

func main() {
	apiUrl := os.Getenv("NEUTRON_API_URL")
	fullJobName := os.Getenv("FULL_JOB_NAME")
	jobName := os.Getenv("JOB_NAME")
	triggerType := os.Getenv("TRIGGER")
	webhookType := os.Getenv("RUNNER_PLATFORM")

	skipTLS := strings.EqualFold(os.Getenv("SKIP_TLS_VERIFY"), "true")
	skipTriggerCheck := strings.EqualFold(os.Getenv("SKIP_TRIGGER_CHECK"), "true")

	// Composite: NoOp + Neutron
	noopReporter := NewNoOpReporter()
	neutronReporter := reporter.NewNeutron(apiUrl, fullJobName, triggerType, webhookType, skipTLS)
	neutronReporter.RegisterPod(os.Getenv("POD_NAME"), os.Getenv("POD_NAMESPACE"))
	composite := reporter.NewComposite(noopReporter, neutronReporter)

	runner := service.NewRunner("/repo", triggerType, jobName, composite, skipTriggerCheck)
	runner.Run()
}
