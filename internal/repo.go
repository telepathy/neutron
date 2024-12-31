package internal

import (
	"database/sql"
	_ "embed"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"neutron/internal/model"
)

type WebhookConfig struct {
	ProjectUrl  string
	WebhookType string
	RepoUrl     string
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

func (r *Repository) GetWebhookConfig(id string) WebhookConfig {
	row, err := r.db.Query("SELECT webhook_type,project_url,repo_url FROM project WHERE id=?", id)
	defer row.Close()
	var webhookConfig WebhookConfig
	if err != nil {
		return webhookConfig
	}
	for row.Next() {
		err = row.Scan(&webhookConfig.WebhookType, &webhookConfig.ProjectUrl, &webhookConfig.RepoUrl)
		break
	}
	return webhookConfig
}
