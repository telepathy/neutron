package main

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"net/http"
	"neutron/internal"
	"neutron/internal/model"
	"neutron/internal/parser"
	"os"
)

func main() {
	var config model.Config
	data, _ := os.ReadFile("./config.yaml")
	_ = yaml.Unmarshal(data, &config)
	repo := internal.NewRepository(config)
	r := gin.Default()
	r.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhookConfig := repo.GetWebhookConfig(id)
		var pipeline model.Pipeline
		var err error
		switch webhookConfig.WebhookType {
		case "GitLab":
			p := parser.NewGitLabParser(c.Request.Body, config.BaseConfig["GitLab"].Url, config.BaseConfig["GitLab"].Token)
			pipeline, err = p.Parse()
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "pipeline": pipeline})
	})

	_ = r.Run(":8888")
}
