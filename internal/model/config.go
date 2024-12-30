package model

type Config struct {
	Port       int                 `yaml:"port"`
	Database   string              `yaml:"database"`
	Salt       string              `yaml:"salt"`
	BaseConfig map[string]CodeBase `yaml:"codebase"`
}

type CodeBase struct {
	Url   string `yaml:"url"`
	Token string `json:"token"`
}

type TriggerType string

const (
	MR   TriggerType = "MR"
	TAG  TriggerType = "TAG"
	PUSH TriggerType = "PUSH"
)
