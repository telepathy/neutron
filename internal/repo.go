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

type PipelineLog struct {
	Id      int
	JobName string
	PodName string
	Log     string
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

type PodStatus struct {
	Name   string
	Status string
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
	row, err := r.db.Query("SELECT webhook_type,id,repo_url FROM project WHERE id=?", id)
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
	_, err := r.db.Exec("INSERT INTO project(id, webhook_type, repo_url) VALUES (?, ?, ?)", p.Id, p.WebhookType, p.RepoUrl)
	return err
}

func (r *Repository) AddJob(job PipelineJob) error {
	_, err := r.db.Exec("INSERT INTO job(project_id, name, status) VALUES(?, ?, ?)", job.ProjectId, job.Name, job.Status)
	return err
}

func (r *Repository) UpdateJobStatus(jobName string, status JobStatus) error {
	statusBytes, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = r.db.Exec("UPDATE job SET status=? WHERE name=?", string(statusBytes), jobName)
	return err
}

func (r *Repository) GetJobStatus(jobName string) (JobStatus, error) {
	row, err := r.db.Query("SELECT status FROM job WHERE name=?", jobName)
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

func (r *Repository) AddPodLog(jobName string, podName string, content string, status string) error {
	_, err := r.db.Exec("INSERT INTO log (job_name, pod_name, content, status) VALUES(?, ?, ?, ?)", jobName, podName, content, status)
	return err
}

func (r *Repository) GetPodStatus(jobName string) ([]PodStatus, error) {
	row, err := r.db.Query("SELECT pod_name, status FROM log WHERE job_name=?", jobName)
	if err != nil {
		return nil, err
	}
	defer row.Close()
	var podStatus []PodStatus
	for row.Next() {
		var p PodStatus
		if err := row.Scan(&p.Name, &p.Status); err != nil {
			return nil, err
		}
		podStatus = append(podStatus, p)
	}
	return podStatus, nil
}

func (r *Repository) GetLogs(podName string) (string, error) {
	row, err := r.db.Query("SELECT content FROM log WHERE pod_name=?", podName)
	if err != nil {
		return "", err
	}
	defer row.Close()
	var content string
	for row.Next() {
		if err := row.Scan(&content); err != nil {
			return "", err
		}
		break
	}
	return content, nil
}
