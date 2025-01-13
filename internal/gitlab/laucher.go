package gitlab

import (
	"fmt"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

type RunnerConfig struct {
	GitlabToken   string
	GitlabUrl     string
	ProjectId     string
	CommitSha     string
	ReportSha     string
	Trigger       string
	JobName       string
	GitRepoUrl    string
	GitPrivateKey string
	PipelineUrl   string
}

type Launcher struct {
	KubeConfigPath string
	Namespace      string
	RunnerConfig   RunnerConfig
	InitImage      string
	PipelineImage  string
	SshKeyName     string
}

func NewGitLabLauncher(kubeConfigPath string, namespace string, runnerConfig RunnerConfig, initImage string, baseImage string, keyName string) *Launcher {
	return &Launcher{
		KubeConfigPath: kubeConfigPath,
		Namespace:      namespace,
		RunnerConfig:   runnerConfig,
		InitImage:      initImage,
		PipelineImage:  baseImage,
		SshKeyName:     keyName,
	}
}

func (l *Launcher) CreateJob(neutronHost string) *batchv1.Job {
	ts := time.Now().Format("20060102-150405")
	var checkoutCommand string
	if l.RunnerConfig.Trigger == "MR" {
		// for mr, fetch gitlab mr ref then checkout ref
		checkoutCommand = fmt.Sprintf("git clone %s /repo && git fetch origin %s:%s && git checkout %s",
			l.RunnerConfig.GitRepoUrl, l.RunnerConfig.CommitSha, l.RunnerConfig.CommitSha, l.RunnerConfig.CommitSha)
	} else {
		// for tag or push, checkout specific sha
		checkoutCommand = fmt.Sprintf("git clone %s /repo && git checkout %s", l.RunnerConfig.GitRepoUrl, l.RunnerConfig.CommitSha)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("neutron-%s-%s", l.RunnerConfig.JobName, ts),
			Namespace: l.Namespace,
			Annotations: map[string]string{
				"sourceType":  "GitLab",
				"sourceLink":  fmt.Sprintf("%s/projects/%s", l.RunnerConfig.GitlabUrl, l.RunnerConfig.ProjectId),
				"triggerType": l.RunnerConfig.Trigger,
				"gitPath":     l.RunnerConfig.GitRepoUrl,
			},
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "pipeline",
							Image:   l.PipelineImage,
							Command: []string{"/pipeline/runner"},
							Env: []v1.EnvVar{
								{Name: "GIT_REPO_URL", Value: l.RunnerConfig.GitRepoUrl},
								{Name: "GIT_PRIVATE_KEY", Value: l.RunnerConfig.GitPrivateKey},
								{Name: "GITLAB_COMMIT_SHA", Value: l.RunnerConfig.CommitSha},
								{Name: "GITLAB_REPORT_SHA", Value: l.RunnerConfig.ReportSha},
								{Name: "GITLAB_PROJECT_ID", Value: l.RunnerConfig.ProjectId},
								{Name: "GITLAB_TOKEN", Value: l.RunnerConfig.GitlabToken},
								{Name: "TRIGGER", Value: l.RunnerConfig.Trigger},
								{Name: "GITLAB_URL", Value: l.RunnerConfig.GitlabUrl},
								{Name: "JOB_NAME", Value: l.RunnerConfig.JobName},
								{Name: "PIPELINE_URL", Value: fmt.Sprintf("%s/status/neutron-%s-%s", neutronHost, l.RunnerConfig.JobName, ts)},
							},
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
								"/bin/sh",
								"-c",
								fmt.Sprintf("curl -k -o /pipeline/runner %s/runner-bin/gitlab && chmod a+x /pipeline/runner", neutronHost),
							},
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
