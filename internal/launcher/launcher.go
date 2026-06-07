package launcher

import (
	"fmt"
	"strings"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"neutron/internal/model"
	"time"
)

type Launcher struct {
	Namespace      string
	RunnerConfig   model.RunnerConfig
	InitImage      string
	PipelineImage  string
	SshKeyName     string
	ExtraEnv       []v1.EnvVar // platform-specific env vars (e.g. TARGET_BRANCH for GitLab MR)
}

func NewLauncher(namespace string, runnerConfig model.RunnerConfig, initImage string, baseImage string, keyName string, extraEnv ...v1.EnvVar) *Launcher {
	return &Launcher{
		Namespace:      namespace,
		RunnerConfig:   runnerConfig,
		InitImage:      initImage,
		PipelineImage:  baseImage,
		SshKeyName:     keyName,
		ExtraEnv:       extraEnv,
	}
}

func (l *Launcher) CreateJob(neutronHost string) *batchv1.Job {
	ts := time.Now().Format("20060102-150405")
	var checkoutCommand string
	if l.RunnerConfig.Trigger == "MR" {
		// clone target branch, fetch source commit, merge
		checkoutCommand = fmt.Sprintf(
			"git clone --branch %s %s /repo && cd /repo && git config user.email neutron@ci && git config user.name neutron && git fetch origin %s && git merge --no-edit %s",
			shellEscape(l.RunnerConfig.TargetBranch), shellEscape(l.RunnerConfig.GitRepoUrl),
			shellEscape(l.RunnerConfig.CommitSha), shellEscape(l.RunnerConfig.CommitSha))
	} else {
		// for tag or push, checkout specific sha
		checkoutCommand = fmt.Sprintf("git clone %s /repo && git checkout %s",
			shellEscape(l.RunnerConfig.GitRepoUrl), shellEscape(l.RunnerConfig.CommitSha))
	}

	// common env vars for all platforms
	env := []v1.EnvVar{
		{Name: "CODEBASE_TOKEN", Value: l.RunnerConfig.CodebaseToken},
		{Name: "CODEBASE_URL", Value: l.RunnerConfig.CodebaseUrl},
		{Name: "PROJECT_ID", Value: l.RunnerConfig.ProjectId},
		{Name: "COMMIT_SHA", Value: l.RunnerConfig.CommitSha},
		{Name: "REPORT_SHA", Value: l.RunnerConfig.ReportSha},
		{Name: "TRIGGER", Value: l.RunnerConfig.Trigger},
		{Name: "JOB_NAME", Value: l.RunnerConfig.JobName},
		{Name: "GIT_REPO_URL", Value: l.RunnerConfig.GitRepoUrl},
		{Name: "GIT_PRIVATE_KEY", Value: l.RunnerConfig.GitPrivateKey},
		{Name: "PIPELINE_URL", Value: fmt.Sprintf("%s/status/neutron-%s-%s", neutronHost, l.RunnerConfig.JobName, ts)},
	}
	env = append(env, l.ExtraEnv...)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("neutron-%s-%s", l.RunnerConfig.JobName, ts),
			Namespace: l.Namespace,
			Annotations: map[string]string{
				"sourceLink":  fmt.Sprintf("%s/projects/%s", l.RunnerConfig.CodebaseUrl, l.RunnerConfig.ProjectId),
				"triggerType": l.RunnerConfig.Trigger,
				"gitPath":     l.RunnerConfig.GitRepoUrl,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32Ptr(0),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "pipeline",
							Image:   l.PipelineImage,
							Command: []string{"/pipeline/runner"},
							Env:     env,
							VolumeMounts: []v1.VolumeMount{
								{MountPath: "/pipeline", Name: "pipeline"},
								{MountPath: "/repo", Name: "repo"},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:  "checkout",
							Image: l.PipelineImage,
							Command: []string{
								"/bin/sh",
								"-c",
								checkoutCommand,
							},
							WorkingDir: "/repo",
							Env: []v1.EnvVar{
								{Name: "GIT_SSH_COMMAND", Value: "ssh -o StrictHostKeyChecking=no"},
							},
							VolumeMounts: []v1.VolumeMount{
								{MountPath: "/repo", Name: "repo"},
								{MountPath: "/root/.ssh/id_rsa", Name: "private-key", SubPath: "id_rsa", ReadOnly: true},
							},
						},
						{
							Name:  "init",
							Image: l.InitImage,
							Command: []string{
								"/bin/sh", "-c",
								`case "${RUNNER_PLATFORM}" in codeup) cp /runners/codeup-runner /pipeline/runner ;; *) cp /runners/gitlab-runner /pipeline/runner ;; esac`,
							},
							Env: env,
							VolumeMounts: []v1.VolumeMount{
								{MountPath: "/pipeline", Name: "pipeline"},
							},
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{Name: "pipeline", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
						{Name: "repo", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
						{Name: "private-key", VolumeSource: v1.VolumeSource{
							Secret: &v1.SecretVolumeSource{
								SecretName: l.SshKeyName,
								Items: []v1.KeyToPath{
									{Key: "id_rsa", Path: "id_rsa"},
								},
								DefaultMode: int32Ptr(0400),
							},
						},
						},
					},
				},
			},
		},
	}
	return job
}

func int32Ptr(i int32) *int32 {
	return &i
}

// shellEscape wraps a string in single quotes for safe use in shell commands.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
