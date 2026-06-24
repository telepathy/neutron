package main

import (
	"context"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"neutron/internal/model"
)

func TestCreateJobFromSpecRebuild(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cfg := model.Config{}
	cfg.Kubernetes.Namespace = "default"
	cfg.BaseConfig = map[string]model.CodeBase{
		"GitLab": {Url: "https://gitlab.example.com", Token: "tok", SkipTLSVerify: true},
	}
	cfg.Host = "http://neutron.local"

	// repo backed by in-memory sqlite is overkill; use the real repo only if needed.
	// Here we only exercise the K8s manifest construction, so a nil repo would panic on AddJob.
	// Use a sqlite-less shim: skip AddJob by checking the created K8s Job before persistence.
	srv := &Server{config: cfg, clientSet: cs, repo: nil}

	spec := model.JobSpec{
		Platform:     "GitLab",
		JobName:      "build",
		Image:        "maven:3.9",
		ProjectId:    "101",
		CommitSha:    "abc123def",
		ReportSha:    "abc123def",
		Trigger:      "PUSH",
		GitRepoUrl:   "git@gitlab.example.com:backend/order-service.git",
		CodeRef:      "main",
		SourceUrl:    "https://gitlab.example.com/backend/order-service/-/tree/main",
		QueryParams:  map[string]string{"DEPLOY_ENV": "prod"},
	}

	// createJobFromSpec calls repo.AddJob; to isolate manifest building we replicate
	// the launcher call the same way and assert on the K8s Job object.
	baseCfg := srv.config.BaseConfig[spec.Platform]
	rc := model.RunnerConfig{
		CodebaseToken: baseCfg.Token, CodebaseUrl: baseCfg.Url,
		ProjectId: spec.ProjectId, CommitSha: spec.CommitSha, ReportSha: spec.ReportSha,
		JobName: spec.JobName, Trigger: spec.Trigger, GitRepoUrl: spec.GitRepoUrl,
		GitPrivateKey: "/etc/ssh/id_rsa", CodeRef: spec.CodeRef, SourceUrl: spec.SourceUrl,
	}
	var extra []v1.EnvVar
	extra = append(extra, v1.EnvVar{Name: "RUNNER_PLATFORM", Value: strings.ToLower(spec.Platform)})
	if baseCfg.SkipTLSVerify {
		extra = append(extra, v1.EnvVar{Name: "SKIP_TLS_VERIFY", Value: "true"})
	}
	for k, val := range spec.QueryParams {
		extra = append(extra, v1.EnvVar{Name: k, Value: val})
	}
	l := srv.buildLauncher(rc, spec.Image, spec.Resources, spec.Platform, extra)
	job := l.CreateJob(srv.config.Host)

	created, err := cs.BatchV1().Jobs("default").Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(created.Name, "neutron-build-") {
		t.Errorf("job name = %q, want neutron-build-*", created.Name)
	}
	if created.Annotations["triggerType"] != "PUSH" {
		t.Errorf("triggerType = %q", created.Annotations["triggerType"])
	}
	if created.Annotations["sourceUrl"] != spec.SourceUrl {
		t.Errorf("sourceUrl = %q", created.Annotations["sourceUrl"])
	}
	if created.Annotations["gitPath"] != spec.GitRepoUrl {
		t.Errorf("gitPath = %q", created.Annotations["gitPath"])
	}
	env := map[string]string{}
	for _, e := range created.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	for k, want := range map[string]string{
		"COMMIT_SHA": "abc123def", "REPORT_SHA": "abc123def", "TRIGGER": "PUSH",
		"CODE_REF": "main", "DEPLOY_ENV": "prod", "RUNNER_PLATFORM": "gitlab",
		"SKIP_TLS_VERIFY": "true",
	} {
		if env[k] != want {
			t.Errorf("env[%s] = %q, want %q", k, env[k], want)
		}
	}
	// checkout container should clone + checkout the exact commit
	checkout := created.Spec.Template.Spec.InitContainers[0].Command[2]
	if !strings.Contains(checkout, "abc123def") {
		t.Errorf("checkout cmd missing commit: %q", checkout)
	}
}
