package internal

import (
	_ "embed"
	"encoding/json"
	"log"
	"neutron/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PipelineProject struct {
	Id          string `gorm:"column:id;primaryKey"`
	WebhookType string `gorm:"column:webhook_type"`
	RepoUrl     string `gorm:"column:repo_url"`
}

func (PipelineProject) TableName() string {
	return "neutron_project"
}

type PipelineJob struct {
	Id        int    `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectId string `gorm:"column:project_id"`
	Name      string `gorm:"column:name"`
	Status    string `gorm:"column:status"`
}

func (PipelineJob) TableName() string {
	return "neutron_job"
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
	db *gorm.DB
}

func NewRepository(config model.Config) *Repository {
	db, err := gorm.Open(mysql.Open(config.Database), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&PipelineProject{}, &PipelineJob{}); err != nil {
		log.Fatalf("failed to auto-migrate database: %v", err)
	}

	return &Repository{
		db: db,
	}
}

func (r *Repository) Close() {
	sqlDB, err := r.db.DB()
	if err != nil {
		return
	}
	sqlDB.Close()
}

func (r *Repository) GetWebhookConfig(id string) PipelineProject {
	var project PipelineProject
	result := r.db.Where("id = ?", id).First(&project)
	if result.Error != nil {
		return PipelineProject{}
	}
	return project
}

func (r *Repository) AddWebhookConfig(p PipelineProject) error {
	return r.db.Create(&p).Error
}

func (r *Repository) ListProjects() ([]PipelineProject, error) {
	var projects []PipelineProject
	err := r.db.Order("id").Find(&projects).Error
	return projects, err
}

func (r *Repository) AddJob(job PipelineJob) error {
	return r.db.Create(&job).Error
}

func (r *Repository) UpdateJobStatus(jobName string, status JobStatus) error {
	statusBytes, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return r.db.Model(&PipelineJob{}).Where("name = ?", jobName).Update("status", string(statusBytes)).Error
}

func (r *Repository) GetJobStatus(jobName string) (JobStatus, error) {
	var job PipelineJob
	result := r.db.Where("name = ?", jobName).First(&job)
	if result.Error != nil {
		return JobStatus{}, result.Error
	}
	var status JobStatus
	if err := json.Unmarshal([]byte(job.Status), &status); err != nil {
		return JobStatus{}, err
	}
	return status, nil
}
