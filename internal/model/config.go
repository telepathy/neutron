package model

type Config struct {
	Host       string              `yaml:"host"`
	Port       int                 `yaml:"port"`
	Database   string              `yaml:"database"`
	Salt       string              `yaml:"salt"`
	LogUrl     string              `yaml:"log_url,omitempty"` // 日志平台链接模板，支持 {namespace} 和 {podName} 占位符
	BaseConfig map[string]CodeBase `yaml:"codebase"`
	// PodCodeBase 覆盖 K8s Pod 内 runner 访问 codebase 的地址（当 Pod 网络与宿主机不同时使用）
	PodCodeBase map[string]CodeBase `yaml:"pod_codebase,omitempty"`
	Kubernetes  KubernetesConfig    `yaml:"kubernetes"`
}

type KubernetesConfig struct {
	KubeConfig    string `yaml:"kube-config"`
	Namespace     string `yaml:"namespace"`
	GitPrivateKey string `yaml:"git-private-key"`
	InitImage     string `yaml:"init-image"`
}

type CodeBase struct {
	Url   string `yaml:"url"`
	Token string `yaml:"token"`
}
