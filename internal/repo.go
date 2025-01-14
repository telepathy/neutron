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
	row, err := r.db.Query("SELECT webhook_type,id,repo_url FROM project WHERE id=?", id)
	defer row.Close()
	var webhookConfig PipelineProject
	if err != nil {
		return webhookConfig
	}
	for row.Next() {
		err = row.Scan(&webhookConfig.WebhookType, &webhookConfig.Id, &webhookConfig.RepoUrl)
		break
	}
	return webhookConfig
}

func (r *Repository) AddJob(job PipelineJob) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	_, err = r.db.Exec("INSERT INTO job(project_id, name, status) VALUES(?, ?, ?)", job.ProjectId, job.Name, job.Status)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	return err
}

func (r *Repository) UpdateJobStatus(jobName string, status JobStatus) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	statusBytes, err := json.Marshal(status)
	_, err = r.db.Exec("UPDATE job SET status=? WHERE name=?", string(statusBytes), jobName)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
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
		err = row.Scan(&statusString)
		break
	}
	var result JobStatus
	err = json.Unmarshal([]byte(statusString), &result)
	return result, err
}

func (r *Repository) AddPodLog(jobName string, podName string, content string, status string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	_, err = r.db.Exec("INSERT INTO log (job_name, pod_name, content, status) VALUES(?, ?, ?, ?)", jobName, podName, content, status)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	return err
}

func (r *Repository) GetPodStatus(jobName string) ([]PodStatus, error) {
	row, err := r.db.Query("SELECT pod_name, status FROM log WHERE job_name=?", jobName)
	if err != nil {
		return nil, err
	}
	defer row.Close()
	podStatus := make([]PodStatus, 0)
	for row.Next() {
		var p PodStatus
		err = row.Scan(&p.Name, &p.Status)
		if err != nil {
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
	content := ""
	for row.Next() {
		err = row.Scan(&content)
		break
	}
	return content, err
}
