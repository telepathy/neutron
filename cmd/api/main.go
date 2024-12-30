package main

import (
	"embed"
	"fmt"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"net/http"
	"neutron/internal"
	"neutron/internal/gitlab"
	"neutron/internal/model"
	"os"
)

//go:embed files/*
var embeddedFiles embed.FS

func main() {
	var config model.Config
	data, _ := os.ReadFile("./config.yaml")
	_ = yaml.Unmarshal(data, &config)
	repo := internal.NewRepository(config)
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhookConfig := repo.GetWebhookConfig(id)
		var pipeline model.Pipeline
		var err error
		switch webhookConfig.WebhookType {
		case "GitLab":
			p := gitlab.NewGitLabParser(c.Request.Body, config.BaseConfig["GitLab"].Url, config.BaseConfig["GitLab"].Token)
			pipeline, err = p.Parse()
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "pipeline": pipeline})
	})

	r.GET("/runner-bin/:type", func(c *gin.Context) {
		runnerBinFile := fmt.Sprintf("files/neutron-%s-runner", c.Param("type"))
		file, err := embeddedFiles.Open(runnerBinFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		defer file.Close()

		fileContent, err := embeddedFiles.ReadFile(runnerBinFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		c.Header("Content-Disposition", "attachment; filename=\"runner\"")
		c.Data(http.StatusOK, "application/octet-stream", fileContent)
	})

	_ = r.Run(fmt.Sprintf(":%d", config.Port))
}
