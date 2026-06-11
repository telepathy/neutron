package internal

import (
	_ "embed"
	"encoding/json"
	"log"
	"neutron/internal/model"
	"time"

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
	Id          int64        `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectId   string       `gorm:"column:project_id"`
	Name        string       `gorm:"column:name;type:varchar(255);uniqueIndex"`
	Status      string       `gorm:"column:status;type:text"`
	Completed   bool         `gorm:"column:completed;default:false"`
	CompletedAt *time.Time   `gorm:"column:completed_at"`
	Pods        []PipelinePod `gorm:"foreignKey:JobId"`
}

func (PipelineJob) TableName() string {
	return "neutron_job"
}

type PipelinePod struct {
	Id     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	JobId  int64  `gorm:"column:job_id;index"`
	PodName string `gorm:"column:pod_name;type:varchar(255)"`
	PodUid  string `gorm:"column:pod_uid;type:varchar(255)"`
	Phase   string `gorm:"column:phase;type:varchar(50)"`
}

func (PipelinePod) TableName() string {
	return "neutron_pod"
}

type NotifyRecipient struct {
	Id        int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectId string     `gorm:"column:project_id;type:char(36);uniqueIndex:idx_project_user" json:"project_id"`
	UserId    string     `gorm:"column:user_id;type:varchar(100);uniqueIndex:idx_project_user" json:"user_id"`
	CreatedAt *time.Time `gorm:"column:created_at" json:"created_at"`
}

func (NotifyRecipient) TableName() string {
	return "neutron_notify"
}

type CCWebhook struct {
	Id          int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectId   string     `gorm:"column:project_id;type:char(36);index" json:"project_id"`
	WebhookUrl  string     `gorm:"column:webhook_url;type:varchar(500)" json:"webhook_url"`
	Description string     `gorm:"column:description;type:varchar(200)" json:"description"`
	CreatedAt   *time.Time `gorm:"column:created_at" json:"created_at"`
}

func (CCWebhook) TableName() string {
	return "neutron_ccwebhook"
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
		Logger: logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&PipelineProject{}, &PipelineJob{}, &PipelinePod{}, &NotifyRecipient{}, &CCWebhook{}); err != nil {
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

func (r *Repository) DB() *gorm.DB {
	return r.db
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

func (r *Repository) ListProjectJobs(projectId string, days int) ([]PipelineJob, error) {
	var jobs []PipelineJob
	err := r.db.Where("project_id = ? AND name >= ?", projectId, time.Now().AddDate(0, 0, -days).Format("20060102")).
		Order("id DESC").Preload("Pods").Find(&jobs).Error
	return jobs, err
}

func (r *Repository) ListAllRecentJobs(days int) ([]PipelineJob, error) {
	var jobs []PipelineJob
	err := r.db.Where("name >= ?", time.Now().AddDate(0, 0, -days).Format("20060102")).
		Order("id DESC").Preload("Pods").Find(&jobs).Error
	return jobs, err
}

func (r *Repository) GetJobByName(name string) (*PipelineJob, error) {
	var job PipelineJob
	result := r.db.Where("name = ?", name).Preload("Pods").First(&job)
	if result.Error != nil {
		return nil, result.Error
	}
	return &job, nil
}

func (r *Repository) AddPod(pod PipelinePod) error {
	return r.db.Create(&pod).Error
}

func (r *Repository) UpdatePodStatus(podUid string, phase string) error {
	return r.db.Model(&PipelinePod{}).Where("pod_uid = ?", podUid).Update("phase", phase).Error
}

func (r *Repository) GetJobPods(jobId int) ([]PipelinePod, error) {
	var pods []PipelinePod
	err := r.db.Where("job_id = ?", jobId).Find(&pods).Error
	return pods, err
}

func (r *Repository) MarkJobCompleted(jobName string) error {
	now := time.Now()
	return r.db.Model(&PipelineJob{}).Where("name = ?", jobName).
		Updates(map[string]interface{}{
			"completed":    true,
			"completed_at": now,
		}).Error
}

func (r *Repository) ListNotifyRecipients(projectId string) ([]NotifyRecipient, error) {
	var recipients []NotifyRecipient
	err := r.db.Where("project_id = ?", projectId).Order("id").Find(&recipients).Error
	return recipients, err
}

func (r *Repository) AddNotifyRecipient(recipient NotifyRecipient) error {
	return r.db.Create(&recipient).Error
}

func (r *Repository) RemoveNotifyRecipient(projectId string, id int64) error {
	return r.db.Where("project_id = ? AND id = ?", projectId, id).Delete(&NotifyRecipient{}).Error
}

func (r *Repository) ListCCWebhooks(projectId string) ([]CCWebhook, error) {
	var webhooks []CCWebhook
	err := r.db.Where("project_id = ?", projectId).Order("id").Find(&webhooks).Error
	return webhooks, err
}

func (r *Repository) AddCCWebhook(webhook CCWebhook) error {
	return r.db.Create(&webhook).Error
}

func (r *Repository) RemoveCCWebhook(projectId string, id int64) error {
	return r.db.Where("project_id = ? AND id = ?", projectId, id).Delete(&CCWebhook{}).Error
}
