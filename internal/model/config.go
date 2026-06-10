package model

type Config struct {
	Host       string              `yaml:"host"`
	Port       int                 `yaml:"port"`
	Database   string              `yaml:"database"`
	Salt       string              `yaml:"salt"`
	LogUrl     string              `yaml:"log_url,omitempty"` // 日志平台链接模板，支持 {namespace} 和 {podId} 占位符
	BaseConfig map[string]CodeBase `yaml:"codebase"`
	// PodCodeBase 覆盖 K8s Pod 内 runner 访问 codebase 的地址（当 Pod 网络与宿主机不同时使用）
	PodCodeBase map[string]CodeBase `yaml:"pod_codebase,omitempty"`
	Kubernetes  KubernetesConfig    `yaml:"kubernetes"`
	Notify      NotifyConfig        `yaml:"notify,omitempty"`
}

type NotifyConfig struct {
	Url   string `yaml:"url"`
	Token string `yaml:"token,omitempty"`
}

type KubernetesConfig struct {
	KubeConfig       string   `yaml:"kube-config"`
	Namespace        string   `yaml:"namespace"`
	GitPrivateKey    string   `yaml:"git-private-key"`
	InitImage        string   `yaml:"init-image"`
	CheckoutImage    string   `yaml:"checkout-image"`      // dedicated image for git checkout (must include git + ssh)
	ImagePullSecrets []string `yaml:"image-pull-secrets,omitempty"` // K8s image pull secret names
	PodApiUrl        string   `yaml:"pod-api-url,omitempty"`        // Pod 内访问 API server 的地址（本地开发用，覆盖集群内地址）
}

type CodeBase struct {
	Url            string `yaml:"url"`
	Token          string `yaml:"token"`
	SkipTLSVerify  bool   `yaml:"skip_tls_verify,omitempty"`
}
