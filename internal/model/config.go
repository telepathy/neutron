package model

type Config struct {
	Host       string              `yaml:"host"`
	Port       int                 `yaml:"port"`
	Database   string              `yaml:"database"`
	Salt       string              `yaml:"salt"`
	BaseConfig map[string]CodeBase `yaml:"codebase"`
	Kubernetes KubernetesConfig    `yaml:"kubernetes"`
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
