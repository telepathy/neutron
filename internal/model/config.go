package model

type Config struct {
	Database string `json:"database"`
}

type TriggerType string

const (
	MR   TriggerType = "MR"
	TAG  TriggerType = "TAG"
	PUSH TriggerType = "PUSH"
)
