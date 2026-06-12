package main

import (
	"log"
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
	repoUrl := os.Getenv("GIT_REPO_URL")

	skipTLS := strings.EqualFold(os.Getenv("SKIP_TLS_VERIFY"), "true")
	skipTriggerCheck := strings.EqualFold(os.Getenv("SKIP_TRIGGER_CHECK"), "true")

	// GitLab reporter for commit statuses
	gitlabReporter, err := NewGitlabReporterFromEnv(skipTLS)
	if err != nil {
		log.Fatalf("failed to create gitlab reporter: %v", err)
	}

	// Composite: GitLab + Neutron
	neutronReporter := reporter.NewNeutron(apiUrl, fullJobName, triggerType, webhookType, repoUrl, skipTLS)
	neutronReporter.RegisterPod(os.Getenv("POD_NAME"), os.Getenv("POD_NAMESPACE"))
	composite := reporter.NewComposite(gitlabReporter, neutronReporter)

	runner := service.NewRunner("/repo", triggerType, jobName, composite, skipTriggerCheck)
	runner.Run()
}
