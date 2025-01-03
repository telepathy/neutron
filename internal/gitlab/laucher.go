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
								{MountPath: l.RunnerConfig.GitPrivateKey, Name: "private-key", SubPath: "id_rsa", ReadOnly: true},
							},
						},
					},
					InitContainers: []v1.Container{
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
