package main

import (
	"context"
	"embed"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"io/fs"
	"log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"neutron/internal"
	"neutron/internal/gitlab"
	"neutron/internal/model"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:embed static/*
var staticFs embed.FS

func main() {
	var config model.Config
	configPath := "./config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "config.yaml"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("cannot read config file: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("cannot parse config file: %v", err)
	}
	repo := internal.NewRepository(config)

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", config.Kubernetes.KubeConfig)
		if err != nil {
			log.Fatalf("cannot build kube config: %v", err)
		}
	}
	clientSet, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	subStaticFs, err := fs.Sub(staticFs, "static")
	if err != nil {
		log.Fatalf("cannot load embedded static files: %v", err)
	}
	r.StaticFS("/static", http.FS(subStaticFs))

	// SPA: serve index.html for all non-API, non-static routes
	r.NoRoute(func(c *gin.Context) {
		data, err := staticFs.ReadFile("static/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "SPA not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// --- API endpoints ---

	r.GET("/api/config", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"logUrl":     config.LogUrl,
			"namespace":  config.Kubernetes.Namespace,
		})
	})

	r.POST("/api/register", func(c *gin.Context) {
		p := internal.PipelineProject{
			Id:          uuid.New().String(),
			WebhookType: c.PostForm("webhookType"),
			RepoUrl:     c.PostForm("repoUrl"),
		}
		err := repo.AddWebhookConfig(p)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fullHost := config.Host
		if config.Port != 80 && config.Port != 443 {
			fullHost = fmt.Sprintf("%s:%d", fullHost, config.Port)
		}
		webhookUrl := fmt.Sprintf("%s/webhook/%s", fullHost, p.Id)
		c.JSON(http.StatusOK, gin.H{
			"id":          p.Id,
			"webhookType": p.WebhookType,
			"repoUrl":     p.RepoUrl,
			"webhookUrl":  webhookUrl,
		})
	})

	r.GET("/api/status/:jobName", func(c *gin.Context) {
		jobName := c.Param("jobName")
		status, err := repo.GetJobStatus(jobName)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{
				"jobName": jobName,
				"status":  status,
				"source":  "database",
			})
			return
		}
		jobClient := clientSet.BatchV1().Jobs(config.Kubernetes.Namespace)
		job, err := jobClient.Get(context.Background(), jobName, metav1.GetOptions{})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		podClient := clientSet.CoreV1().Pods(config.Kubernetes.Namespace)
		selector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchLabels: job.Spec.Selector.MatchLabels,
		})
		pods, err := podClient.List(context.Background(), metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"jobName": jobName,
			"job":     job,
			"pods":    pods,
			"source":  "kubernetes",
		})
	})

	r.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhookConfig := repo.GetWebhookConfig(id)
		if webhookConfig.Id == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		var pipeline model.Pipeline
		var err error
		var jobs []string
		switch webhookConfig.WebhookType {
		case "GitLab":
			p, parseErr := gitlab.NewGitLabParser(c.Request.Body, config.BaseConfig["GitLab"].Url, config.BaseConfig["GitLab"].Token)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": parseErr.Error()})
				return
			}
			pipeline, err = p.Parse()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse pipeline: %v", err)})
				return
			}
			for jobName, job := range pipeline.Jobs {
				if !isValidTrigger(p.Trigger, job.Trigger) {
					continue
				}
				podGitlab := config.BaseConfig["GitLab"]
				if pod, ok := config.PodCodeBase["GitLab"]; ok {
					podGitlab = pod
				}
				runnerConfig := model.RunnerConfig{
					GitlabToken:   podGitlab.Token,
					GitlabUrl:     podGitlab.Url,
					ProjectId:     strconv.Itoa(p.Request.Project.Id),
					CommitSha:     p.CodeSha,
					ReportSha:     p.ReportSha,
					JobName:       jobName,
					Trigger:       p.Trigger,
					GitRepoUrl:    webhookConfig.RepoUrl,
					GitPrivateKey: "/etc/ssh/id_rsa",
					TargetBranch:  p.TargetBranch,
				}
				l := gitlab.NewGitLabLauncher(
					config.Kubernetes.Namespace,
					runnerConfig,
					config.Kubernetes.InitImage,
					job.Image,
					config.Kubernetes.GitPrivateKey,
				)
				jobClient := clientSet.BatchV1().Jobs(config.Kubernetes.Namespace)
				neutronHost := fmt.Sprintf("%s:%d", config.Host, config.Port)
				createdJob, err := jobClient.Create(context.Background(), l.CreateJob(neutronHost), metav1.CreateOptions{})
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				err = repo.AddJob(internal.PipelineJob{
					ProjectId: id,
					Name:      createdJob.Name,
					Status:    "",
				})
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				jobs = append(jobs, createdJob.Name)
			}
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "pipeline": pipeline, "jobs": jobs})
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced shutdown: %v", err)
	}
	log.Println("server exited")

}

func isValidTrigger(currentTrigger string, validTriggers []string) bool {
	for _, trigger := range validTriggers {
		if trigger == currentTrigger {
			return true
		}
	}
	return false
}
