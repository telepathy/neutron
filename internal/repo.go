package internal

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"neutron/internal/model"
)

type PipelineProject struct {
	Id          string
	WebhookType string
	RepoUrl     string
}

type PipelineJob struct {
	Id        int
	ProjectId string
	Name      string
	Status    string
}

type JobStatus struct {
	WebhookType string `json:"webhook_type"`
	RepoUrl     string `json:"repo_url"`
	ProjectUrl  string `json:"project_url"`
	TriggerType string `json:"trigger_type"`
	Active      int    `json:"active"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(config model.Config) *Repository {
	db, err := sql.Open("mysql", config.Database)
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	return &Repository{
		db: db,
	}
}

func (r *Repository) Close() {
	err := r.db.Close()
	if err != nil {
		return
	}
}

func (r *Repository) GetWebhookConfig(id string) PipelineProject {
	var webhookConfig PipelineProject
	row, err := r.db.Query("SELECT webhook_type,id,repo_url FROM neutron_project WHERE id=?", id)
	if err != nil {
		return webhookConfig
	}
	defer row.Close()
	for row.Next() {
		_ = row.Scan(&webhookConfig.WebhookType, &webhookConfig.Id, &webhookConfig.RepoUrl)
		break
	}
	return webhookConfig
}

func (r *Repository) AddWebhookConfig(p PipelineProject) error {
	_, err := r.db.Exec("INSERT INTO neutron_project(id, webhook_type, repo_url) VALUES (?, ?, ?)", p.Id, p.WebhookType, p.RepoUrl)
	return err
}

func (r *Repository) AddJob(job PipelineJob) error {
	_, err := r.db.Exec("INSERT INTO neutron_job(project_id, name, status) VALUES(?, ?, ?)", job.ProjectId, job.Name, job.Status)
	return err
}

func (r *Repository) UpdateJobStatus(jobName string, status JobStatus) error {
	statusBytes, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = r.db.Exec("UPDATE neutron_job SET status=? WHERE name=?", string(statusBytes), jobName)
	return err
}

func (r *Repository) GetJobStatus(jobName string) (JobStatus, error) {
	row, err := r.db.Query("SELECT status FROM neutron_job WHERE name=?", jobName)
	if err != nil {
		return JobStatus{}, err
	}
	defer row.Close()
	var statusString string
	for row.Next() {
		if err := row.Scan(&statusString); err != nil {
			return JobStatus{}, err
		}
		break
	}
	var result JobStatus
	if err := json.Unmarshal([]byte(statusString), &result); err != nil {
		return JobStatus{}, err
	}
	return result, nil
}

