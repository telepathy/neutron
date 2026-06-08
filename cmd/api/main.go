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
	"neutron/internal/codeup"
	"neutron/internal/gitlab"
	"neutron/internal/launcher"
	"neutron/internal/model"
	v1 "k8s.io/api/core/v1"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed static/*
var staticFs embed.FS

func main() {
	var config model.Config
	configPath := os.Getenv("NEUTRON_CONFIG")
	if configPath == "" {
		configPath = "./config.yaml"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("cannot read config file: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("cannot parse config file: %v", err)
	}
	// env overrides
	if v := os.Getenv("NEUTRON_HOST"); v != "" {
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			v = "http://" + v
		}
		config.Host = v
	}
	if v := os.Getenv("NEUTRON_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			config.Port = p
		}
	}
	if v := os.Getenv("NEUTRON_DATABASE"); v != "" {
		config.Database = v
	}
	if v := os.Getenv("NEUTRON_SALT"); v != "" {
		config.Salt = v
	}
	if v := os.Getenv("NEUTRON_LOG_URL"); v != "" {
		config.LogUrl = v
	}
	if v := os.Getenv("NEUTRON_KUBE_NAMESPACE"); v != "" {
		config.Kubernetes.Namespace = v
	}
	if v := os.Getenv("NEUTRON_KUBE_CONFIG"); v != "" {
		config.Kubernetes.KubeConfig = v
	}
	if v := os.Getenv("NEUTRON_GIT_PRIVATE_KEY"); v != "" {
		config.Kubernetes.GitPrivateKey = v
	}
	if v := os.Getenv("NEUTRON_INIT_IMAGE"); v != "" {
		config.Kubernetes.InitImage = v
	}
	if v := os.Getenv("NEUTRON_CHECKOUT_IMAGE"); v != "" {
		config.Kubernetes.CheckoutImage = v
	}
	if v := os.Getenv("NEUTRON_IMAGE_PULL_SECRETS"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				config.Kubernetes.ImagePullSecrets = append(config.Kubernetes.ImagePullSecrets, s)
			}
		}
	}
	if v := os.Getenv("NEUTRON_SKIP_TLS_VERIFY"); v == "true" {
		for k, cb := range config.BaseConfig {
			cb.SkipTLSVerify = true
			config.BaseConfig[k] = cb
		}
	}
	if v := os.Getenv("NEUTRON_GITLAB_URL"); v != "" {
		cb := config.BaseConfig["GitLab"]
		cb.Url = v
		config.BaseConfig["GitLab"] = cb
	}
	if v := os.Getenv("NEUTRON_GITLAB_TOKEN"); v != "" {
		cb := config.BaseConfig["GitLab"]
		cb.Token = v
		config.BaseConfig["GitLab"] = cb
	}
	if v := os.Getenv("NEUTRON_GITLAB_SKIP_TLS_VERIFY"); v == "true" {
		cb := config.BaseConfig["GitLab"]
		cb.SkipTLSVerify = true
		config.BaseConfig["GitLab"] = cb
	}
	if v := os.Getenv("NEUTRON_CODEUP_URL"); v != "" {
		cb := config.BaseConfig["Codeup"]
		cb.Url = v
		config.BaseConfig["Codeup"] = cb
	}
	if v := os.Getenv("NEUTRON_CODEUP_TOKEN"); v != "" {
		cb := config.BaseConfig["Codeup"]
		cb.Token = v
		config.BaseConfig["Codeup"] = cb
	}
	if v := os.Getenv("NEUTRON_CODEUP_SKIP_TLS_VERIFY"); v == "true" {
		cb := config.BaseConfig["Codeup"]
		cb.SkipTLSVerify = true
		config.BaseConfig["Codeup"] = cb
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
		codebaseUrls := make(map[string]string)
		for k, v := range config.BaseConfig {
			codebaseUrls[k] = v.Url
		}
		c.JSON(http.StatusOK, gin.H{
			"logUrl":      config.LogUrl,
			"namespace":   config.Kubernetes.Namespace,
			"codebaseUrls": codebaseUrls,
		})
	})

	r.GET("/api/projects", func(c *gin.Context) {
		projects, err := repo.ListProjects()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"projects": projects})
	})

	r.GET("/api/projects/:id/jobs", func(c *gin.Context) {
		id := c.Param("id")
		jobs, err := repo.ListProjectJobs(id, 7)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"jobs": jobs})
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
		webhookUrl := fmt.Sprintf("%s/webhook/%s", config.Host, p.Id)
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

		// auto-detect platform: X-Codeup-Event header present → Codeup, otherwise use registered type
		platform := webhookConfig.WebhookType
		if c.GetHeader("X-Codeup-Event") != "" {
			platform = "Codeup"
		}

		var pipeline model.Pipeline
		var pTrigger string
		var pCodeSha string
		var pReportSha string
		var pTargetBranch string
		var pProjectId int
		var err error
		var jobs []string

		switch platform {
		case "GitLab":
			p, parseErr := gitlab.NewGitLabParser(c.Request.Body, config.BaseConfig["GitLab"].Url, config.BaseConfig["GitLab"].Token, config.BaseConfig["GitLab"].SkipTLSVerify)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": parseErr.Error()})
				return
			}
			pipeline, err = p.Parse()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse pipeline: %v", err)})
				return
			}
			pTrigger = p.Trigger
			pCodeSha = p.CodeSha
			pReportSha = p.ReportSha
			pTargetBranch = p.TargetBranch
			pProjectId = p.Request.Project.Id
		case "Codeup":
			codeupCfg, ok := config.BaseConfig["Codeup"]
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Codeup codebase not configured"})
				return
			}
			p, parseErr := codeup.NewCodeupParser(c.Request.Body, codeupCfg.Url, codeupCfg.Token, codeupCfg.SkipTLSVerify)
			if parseErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": parseErr.Error()})
				return
			}
			pipeline, err = p.Parse()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse pipeline: %v", err)})
				return
			}
			pTrigger = p.Trigger
			pCodeSha = p.CodeSha
			pProjectId = p.Request.Project.Id
			if pProjectId == 0 {
				pProjectId = p.Request.ProjectId
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported platform: %s", platform)})
			return
		}

		for jobName, job := range pipeline.Jobs {
			if !isValidTrigger(pTrigger, job.Trigger) {
				continue
			}

			// resolve codebase config (pod override if available)
			baseCfg := config.BaseConfig[platform]
			if pod, ok := config.PodCodeBase[platform]; ok {
				baseCfg = pod
			}

			runnerConfig := model.RunnerConfig{
				CodebaseToken: baseCfg.Token,
				CodebaseUrl:   baseCfg.Url,
				ProjectId:     strconv.Itoa(pProjectId),
				CommitSha:     pCodeSha,
				ReportSha:     pReportSha,
				JobName:       jobName,
				Trigger:       pTrigger,
				GitRepoUrl:    webhookConfig.RepoUrl,
				GitPrivateKey: "/etc/ssh/id_rsa",
				TargetBranch:  pTargetBranch,
			}

			// platform-specific extra env vars
			var extraEnv []v1.EnvVar
			extraEnv = append(extraEnv, v1.EnvVar{Name: "RUNNER_PLATFORM", Value: strings.ToLower(platform)})
			if baseCfg.SkipTLSVerify {
				extraEnv = append(extraEnv, v1.EnvVar{Name: "SKIP_TLS_VERIFY", Value: "true"})
			}
			if platform == "GitLab" && pTargetBranch != "" {
				extraEnv = append(extraEnv, v1.EnvVar{Name: "TARGET_BRANCH", Value: pTargetBranch})
			}

			l := launcher.NewLauncher(
				config.Kubernetes.Namespace,
				runnerConfig,
				config.Kubernetes.InitImage,
				config.Kubernetes.CheckoutImage,
				job.Image,
				config.Kubernetes.GitPrivateKey,
				config.Kubernetes.ImagePullSecrets,
				extraEnv...,
			)
			jobClient := clientSet.BatchV1().Jobs(config.Kubernetes.Namespace)
			neutronHost := config.Host
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
