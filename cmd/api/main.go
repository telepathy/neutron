package main

import (
	"context"
	"embed"
	"encoding/json"
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
	"neutron/internal/notify"
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
	if v := os.Getenv("NEUTRON_CODEUP_WEBHOOK_URL"); v != "" {
		cb := config.BaseConfig["Codeup"]
		cb.WebhookUrl = v
		config.BaseConfig["Codeup"] = cb
	}
	if v := os.Getenv("NEUTRON_NOTIFY_URL"); v != "" {
		config.Notify.Url = v
	}
	if v := os.Getenv("NEUTRON_NOTIFY_CORP_ID"); v != "" {
		config.Notify.CorpId = v
	}
	if v := os.Getenv("NEUTRON_NOTIFY_APP_ID"); v != "" {
		config.Notify.AppId = v
	}
	if v := os.Getenv("NEUTRON_NOTIFY_SKIP_TLS_VERIFY"); v == "true" {
		config.Notify.SkipTLSVerify = true
	}
	if v := os.Getenv("NEUTRON_POD_API_URL"); v != "" {
		config.Kubernetes.PodApiUrl = v
	}
	repo := internal.NewRepository(config)

	// Initialize notify client
	var notifyClient *notify.Client
	if config.Notify.Url != "" && config.Notify.CorpId != "" && config.Notify.AppId != "" {
		notifyClient = notify.NewClient(config.Notify.Url, config.Notify.CorpId, config.Notify.AppId, config.Notify.SkipTLSVerify)
	}

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

	r.GET("/api/jobs/recent", func(c *gin.Context) {
		jobs, err := repo.ListAllRecentJobs(7)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"jobs": jobs})
	})

	r.GET("/api/projects/:id/recipients", func(c *gin.Context) {
		id := c.Param("id")
		recipients, err := repo.ListNotifyRecipients(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"recipients": recipients})
	})

	r.POST("/api/projects/:id/recipients", func(c *gin.Context) {
		id := c.Param("id")
		var req struct {
			UserId string `json:"user_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.UserId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}
		// Check duplicate
		var existing internal.NotifyRecipient
		if err := repo.DB().Where("project_id = ? AND user_id = ?", id, req.UserId).First(&existing).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Recipient already exists"})
			return
		}
		recipient := internal.NotifyRecipient{ProjectId: id, UserId: req.UserId}
		if err := repo.AddNotifyRecipient(recipient); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": recipient.Id, "project_id": id, "user_id": req.UserId})
	})

	r.DELETE("/api/projects/:id/recipients/:rid", func(c *gin.Context) {
		id := c.Param("id")
		rid, err := strconv.ParseInt(c.Param("rid"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recipient id"})
			return
		}
		if err := repo.RemoveNotifyRecipient(id, rid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
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
		// Determine webhook URL: use platform-specific URL if configured, otherwise use config.Host
		webhookHost := config.Host
		if cb, ok := config.BaseConfig[p.WebhookType]; ok && cb.WebhookUrl != "" {
			webhookHost = cb.WebhookUrl
		}
		webhookUrl := fmt.Sprintf("%s/webhook/%s", webhookHost, p.Id)
		c.JSON(http.StatusOK, gin.H{
			"id":          p.Id,
			"webhookType": p.WebhookType,
			"repoUrl":     p.RepoUrl,
			"webhookUrl":  webhookUrl,
		})
	})

	r.GET("/api/status/:jobName", func(c *gin.Context) {
		jobName := c.Param("jobName")

		// Check if job is completed in database - if yes, return from DB only
		dbJob, dbErr := repo.GetJobByName(jobName)
		if dbErr == nil && dbJob.Completed {
			var status internal.JobStatus
			_ = json.Unmarshal([]byte(dbJob.Status), &status)
			// Convert pods to K8s-like format for frontend compatibility
			var podItems []gin.H
			for _, pod := range dbJob.Pods {
				podItems = append(podItems, gin.H{
					"metadata": gin.H{"name": pod.PodName, "uid": pod.PodUid},
					"status":   gin.H{"phase": pod.Phase},
				})
			}
			c.JSON(http.StatusOK, gin.H{
				"jobName": jobName,
				"status":  status,
				"job":     gin.H{"metadata": gin.H{"name": jobName}},
				"pods":    gin.H{"items": podItems},
				"source":  "database",
			})
			return
		}

		// Job not completed, fetch from K8s
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

		// Derive status from K8s job
		ann := job.Annotations
		k8sStatus := internal.JobStatus{
			WebhookType: ann["sourceType"],
			TriggerType: ann["triggerType"],
			RepoUrl:     ann["gitPath"],
			ProjectUrl:  ann["sourceLink"],
		}
		if job.Status.Active > 0 {
			k8sStatus.Active = 1
		}
		if job.Status.Succeeded > 0 {
			k8sStatus.Succeeded = 1
		}
		if job.Status.Failed > 0 {
			k8sStatus.Failed = 1
		}
		// Update database with derived status
		_ = repo.UpdateJobStatus(jobName, k8sStatus)

		// Store pod info in database
		if dbErr == nil {
			for _, pod := range pods.Items {
				// Check if pod already exists
				var existingPod internal.PipelinePod
				result := repo.DB().Where("job_id = ? AND pod_uid = ?", dbJob.Id, string(pod.UID)).First(&existingPod)
				if result.Error != nil {
					// Pod doesn't exist, create it
					_ = repo.AddPod(internal.PipelinePod{
						JobId:   dbJob.Id,
						PodName: pod.Name,
						PodUid:  string(pod.UID),
						Phase:   string(pod.Status.Phase),
					})
				} else {
					// Update existing pod status
					_ = repo.UpdatePodStatus(string(pod.UID), string(pod.Status.Phase))
				}
			}
		}

		// If job is completed, mark it in database
		if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
			_ = repo.MarkJobCompleted(jobName)
		}

		c.JSON(http.StatusOK, gin.H{
			"jobName": jobName,
			"status":  k8sStatus,
			"job":     job,
			"pods":    pods,
			"source":  "kubernetes",
		})
	})

	r.POST("/api/report/:jobName", func(c *gin.Context) {
		jobName := c.Param("jobName")
		var status internal.JobStatus
		if err := c.ShouldBindJSON(&status); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		err := repo.UpdateJobStatus(jobName, status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Sync pod phase from K8s on every non-terminal report
		if status.Succeeded == 0 && status.Failed == 0 {
			if dbJob, err := repo.GetJobByName(jobName); err == nil {
				for _, pod := range dbJob.Pods {
					if k8sPod, err := clientSet.CoreV1().Pods(config.Kubernetes.Namespace).Get(context.Background(), pod.PodName, metav1.GetOptions{}); err == nil {
						_ = repo.UpdatePodStatus(pod.PodUid, string(k8sPod.Status.Phase))
					}
				}
			}
		}
		// Mark job completed and asynchronously sync final pod phase
		if status.Succeeded > 0 || status.Failed > 0 {
			// Notify recipients: pipeline completed
			if notifyClient != nil {
				if dbJob, err := repo.GetJobByName(jobName); err == nil {
					if recipients, err := repo.ListNotifyRecipients(dbJob.ProjectId); err == nil {
						statusUrl := fmt.Sprintf("%s/#/status/%s", config.Host, jobName)
						var content string
						if status.Failed > 0 {
							content = fmt.Sprintf("❌ 流水线执行失败\n\n📂 项目: %s\n📋 任务: %s\n🔗 查看: %s", dbJob.ProjectId, jobName, statusUrl)
						} else {
							content = fmt.Sprintf("✅ 流水线执行成功\n\n📂 项目: %s\n📋 任务: %s\n🔗 查看: %s", dbJob.ProjectId, jobName, statusUrl)
						}
						for _, r := range recipients {
							go notifyClient.SendMessage(r.UserId, content)
						}
					}
				}
			}
			finalPhase := "Succeeded"
			if status.Failed > 0 {
				finalPhase = "Failed"
			}
			go func() {
				// Retry until pod reaches terminal phase or is gone
				for i := 0; i < 5; i++ {
					time.Sleep(2 * time.Second)
					if dbJob, err := repo.GetJobByName(jobName); err == nil {
						allSynced := true
						for _, pod := range dbJob.Pods {
							k8sPod, err := clientSet.CoreV1().Pods(config.Kubernetes.Namespace).Get(context.Background(), pod.PodName, metav1.GetOptions{})
							if err != nil {
								// Pod gone — use runner's reported phase
								_ = repo.UpdatePodStatus(pod.PodUid, finalPhase)
								continue
							}
							phase := string(k8sPod.Status.Phase)
							if phase == "Succeeded" || phase == "Failed" {
								_ = repo.UpdatePodStatus(pod.PodUid, phase)
							} else {
								allSynced = false
							}
						}
						if allSynced {
							break
						}
					}
				}
				_ = repo.MarkJobCompleted(jobName)
			}()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/api/report/:jobName/pod", func(c *gin.Context) {
		jobName := c.Param("jobName")

		var req struct {
			PodName   string `json:"pod_name"`
			Namespace string `json:"namespace"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Look up job in database
		dbJob, dbErr := repo.GetJobByName(jobName)
		if dbErr != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}

		// Get pod details from K8s
		podClient := clientSet.CoreV1().Pods(req.Namespace)
		pod, err := podClient.Get(context.Background(), req.PodName, metav1.GetOptions{})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("pod not found: %v", err)})
			return
		}

		// Upsert pod info
		var existingPod internal.PipelinePod
		result := repo.DB().Where("job_id = ? AND pod_uid = ?", dbJob.Id, string(pod.UID)).First(&existingPod)
		if result.Error != nil {
			_ = repo.AddPod(internal.PipelinePod{
				JobId:   dbJob.Id,
				PodName: pod.Name,
				PodUid:  string(pod.UID),
				Phase:   string(pod.Status.Phase),
			})
		} else {
			_ = repo.UpdatePodStatus(string(pod.UID), string(pod.Status.Phase))
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhookConfig := repo.GetWebhookConfig(id)
		if webhookConfig.Id == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}

		platform := webhookConfig.WebhookType

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
			pReportSha = p.ReportSha
			pProjectId = p.Request.Project.Id
			if pProjectId == 0 {
				pProjectId = p.Request.ProjectId
			}
			if pProjectId == 0 {
				pProjectId = p.Request.Attributes.ProjectId
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
				platform,
				config.Kubernetes.PodApiUrl,
				job.Resources,
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

		// Notify recipients: pipeline triggered
		if len(jobs) > 0 && notifyClient != nil {
			if recipients, err := repo.ListNotifyRecipients(id); err == nil {
				statusUrl := fmt.Sprintf("%s/#/status/%s", config.Host, jobs[0])
				content := fmt.Sprintf("🚀 流水线触发通知\n\n📂 项目: %s\n🔄 触发: %s\n🔗 查看: %s", webhookConfig.RepoUrl, pTrigger, statusUrl)
				for _, r := range recipients {
					go notifyClient.SendMessage(r.UserId, content)
				}
			}
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
