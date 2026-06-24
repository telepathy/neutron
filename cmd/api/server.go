package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"neutron/internal"
	"neutron/internal/ccwork"
	"neutron/internal/codeup"
	"neutron/internal/gitlab"
	"neutron/internal/launcher"
	"neutron/internal/model"
	"neutron/internal/notify"
	"neutron/internal/parser"
)

// Server holds the dependencies shared by all HTTP handlers.
type Server struct {
	config       model.Config
	repo         *internal.Repository
	clientSet    kubernetes.Interface
	notifyClient *notify.Client
	ccworkRobot  *ccwork.Robot
}

// NewServer wires the server dependencies together.
func NewServer(config model.Config, repo *internal.Repository, clientSet kubernetes.Interface, notifyClient *notify.Client, ccworkRobot *ccwork.Robot) *Server {
	return &Server{
		config:       config,
		repo:         repo,
		clientSet:    clientSet,
		notifyClient: notifyClient,
		ccworkRobot:  ccworkRobot,
	}
}

// registerRoutes registers all API and webhook routes on the given engine.
func (s *Server) registerRoutes(r *gin.Engine) {
	r.GET("/api/config", s.handleConfig)
	r.GET("/api/projects", s.handleListProjects)
	r.GET("/api/projects/:id/jobs", s.handleListProjectJobs)
	r.GET("/api/jobs/recent", s.handleRecentJobs)
	r.POST("/api/register", s.handleRegister)
	r.GET("/api/status/:jobName", s.handleStatus)
	r.POST("/api/report/:jobName", s.handleReport)
	r.POST("/api/report/:jobName/pod", s.handleReportPod)
	r.POST("/api/report/:jobName/link", s.handleReportLink)
	r.POST("/webhook/:id", s.handleWebhook)
	r.POST("/api/trigger", s.handleTrigger)
}

func (s *Server) handleConfig(c *gin.Context) {
	codebaseUrls := make(map[string]string)
	for k, v := range s.config.BaseConfig {
		codebaseUrls[k] = v.Url
	}
	c.JSON(http.StatusOK, gin.H{
		"logUrl":       s.config.LogUrl,
		"namespace":    s.config.Kubernetes.Namespace,
		"codebaseUrls": codebaseUrls,
	})
}

func (s *Server) handleListProjects(c *gin.Context) {
	projects, err := s.repo.ListProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (s *Server) handleListProjectJobs(c *gin.Context) {
	id := c.Param("id")
	jobs, err := s.repo.ListProjectJobs(id, 7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}

func (s *Server) handleRecentJobs(c *gin.Context) {
	jobs, err := s.repo.ListAllRecentJobs(7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}

func (s *Server) handleRegister(c *gin.Context) {
	p := internal.PipelineProject{
		Id:          uuid.New().String(),
		WebhookType: c.PostForm("webhookType"),
		RepoUrl:     c.PostForm("repoUrl"),
	}
	if err := s.repo.AddWebhookConfig(p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Determine webhook URL: use platform-specific URL if configured, otherwise use config.Host
	webhookHost := s.config.Host
	if cb, ok := s.config.BaseConfig[p.WebhookType]; ok && cb.WebhookUrl != "" {
		webhookHost = cb.WebhookUrl
	}
	webhookUrl := fmt.Sprintf("%s/webhook/%s", webhookHost, p.Id)
	c.JSON(http.StatusOK, gin.H{
		"id":          p.Id,
		"webhookType": p.WebhookType,
		"repoUrl":     p.RepoUrl,
		"webhookUrl":  webhookUrl,
	})
}

func (s *Server) handleStatus(c *gin.Context) {
	jobName := c.Param("jobName")

	// Check if job is completed in database - if yes, return from DB only
	dbJob, dbErr := s.repo.GetJobByName(jobName)
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
		var reportUrl string
		if url, err := s.repo.GetJobReportUrl(jobName); err == nil {
			reportUrl = url
		}
		c.JSON(http.StatusOK, gin.H{
			"jobName":   jobName,
			"status":    status,
			"job":       gin.H{"metadata": gin.H{"name": jobName}},
			"pods":      gin.H{"items": podItems},
			"source":    "database",
			"reportUrl": reportUrl,
		})
		return
	}

	// Job not completed, fetch from K8s
	jobClient := s.clientSet.BatchV1().Jobs(s.config.Kubernetes.Namespace)
	job, err := jobClient.Get(context.Background(), jobName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	podClient := s.clientSet.CoreV1().Pods(s.config.Kubernetes.Namespace)
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
		SourceUrl:   ann["sourceUrl"],
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
	_ = s.repo.UpdateJobStatus(jobName, k8sStatus)

	// Store pod info in database
	if dbErr == nil {
		for _, pod := range pods.Items {
			// Check if pod already exists
			var existingPod internal.PipelinePod
			result := s.repo.DB().Where("job_id = ? AND pod_uid = ?", dbJob.Id, string(pod.UID)).First(&existingPod)
			if result.Error != nil {
				// Pod doesn't exist, create it
				_ = s.repo.AddPod(internal.PipelinePod{
					JobId:   dbJob.Id,
					PodName: pod.Name,
					PodUid:  string(pod.UID),
					Phase:   string(pod.Status.Phase),
				})
			} else {
				// Update existing pod status
				_ = s.repo.UpdatePodStatus(string(pod.UID), string(pod.Status.Phase))
			}
		}
	}

	// If job is completed, mark it in database
	if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
		_ = s.repo.MarkJobCompleted(jobName)
	}

	var reportUrl string
	if url, err := s.repo.GetJobReportUrl(jobName); err == nil {
		reportUrl = url
	}
	c.JSON(http.StatusOK, gin.H{
		"jobName":   jobName,
		"status":    k8sStatus,
		"job":       job,
		"pods":      pods,
		"source":    "kubernetes",
		"reportUrl": reportUrl,
	})
}

func (s *Server) handleReport(c *gin.Context) {
	jobName := c.Param("jobName")
	var status internal.JobStatus
	if err := c.ShouldBindJSON(&status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Preserve existing SourceUrl from DB if not provided in report (runners don't send it)
	if status.SourceUrl == "" {
		if oldStatus, err := s.repo.GetJobStatus(jobName); err == nil {
			status.SourceUrl = oldStatus.SourceUrl
		}
	}
	// Fallback: load SourceUrl from K8s Job annotations if still empty
	if status.SourceUrl == "" {
		if k8sJob, err := s.clientSet.BatchV1().Jobs(s.config.Kubernetes.Namespace).Get(context.Background(), jobName, metav1.GetOptions{}); err == nil {
			if srcUrl := k8sJob.Annotations["sourceUrl"]; srcUrl != "" {
				status.SourceUrl = srcUrl
			}
		}
	}
	if err := s.repo.UpdateJobStatus(jobName, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Sync pod phase from K8s on every non-terminal report
	if status.Succeeded == 0 && status.Failed == 0 {
		if dbJob, err := s.repo.GetJobByName(jobName); err == nil {
			for _, pod := range dbJob.Pods {
				if k8sPod, err := s.clientSet.CoreV1().Pods(s.config.Kubernetes.Namespace).Get(context.Background(), pod.PodName, metav1.GetOptions{}); err == nil {
					_ = s.repo.UpdatePodStatus(pod.PodUid, string(k8sPod.Status.Phase))
				}
			}
		}
	}
	// Mark job completed and asynchronously sync final pod phase
	if status.Succeeded > 0 || status.Failed > 0 {
		// Notify recipients: pipeline completed
		if dbJob, err := s.repo.GetJobByName(jobName); err == nil {
			statusUrl := fmt.Sprintf("%s/#/status/%s", s.config.Host, jobName)
			project := s.repo.GetWebhookConfig(dbJob.ProjectId)
			repoUrl := project.RepoUrl
			if repoUrl == "" {
				repoUrl = dbJob.ProjectId
			}
			var title, content string
			if status.Failed > 0 {
				title = "❌ 流水线执行失败"
			} else {
				title = "✅ 流水线执行成功"
			}
			content = fmt.Sprintf("📂 项目: %s\n📋 任务: %s\n🔗 查看: %s", repoUrl, jobName, statusUrl)
			if status.SourceUrl != "" {
				content += fmt.Sprintf("\n📎 源码: %s", status.SourceUrl)
			}
			s.sendJobNotifications(parseNotify(dbJob.Notify), title, content)
		}
		finalPhase := "Succeeded"
		if status.Failed > 0 {
			finalPhase = "Failed"
		}
		go func() {
			// Retry until pod reaches terminal phase or is gone
			for i := 0; i < 5; i++ {
				time.Sleep(2 * time.Second)
				if dbJob, err := s.repo.GetJobByName(jobName); err == nil {
					allSynced := true
					for _, pod := range dbJob.Pods {
						k8sPod, err := s.clientSet.CoreV1().Pods(s.config.Kubernetes.Namespace).Get(context.Background(), pod.PodName, metav1.GetOptions{})
						if err != nil {
							// Pod gone — use runner's reported phase
							_ = s.repo.UpdatePodStatus(pod.PodUid, finalPhase)
							continue
						}
						phase := string(k8sPod.Status.Phase)
						if phase == "Succeeded" || phase == "Failed" {
							_ = s.repo.UpdatePodStatus(pod.PodUid, phase)
						} else {
							allSynced = false
						}
					}
					if allSynced {
						break
					}
				}
			}
			_ = s.repo.MarkJobCompleted(jobName)
		}()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleReportPod(c *gin.Context) {
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
	dbJob, dbErr := s.repo.GetJobByName(jobName)
	if dbErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Get pod details from K8s
	podClient := s.clientSet.CoreV1().Pods(req.Namespace)
	pod, err := podClient.Get(context.Background(), req.PodName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("pod not found: %v", err)})
		return
	}

	// Upsert pod info
	var existingPod internal.PipelinePod
	result := s.repo.DB().Where("job_id = ? AND pod_uid = ?", dbJob.Id, string(pod.UID)).First(&existingPod)
	if result.Error != nil {
		_ = s.repo.AddPod(internal.PipelinePod{
			JobId:   dbJob.Id,
			PodName: pod.Name,
			PodUid:  string(pod.UID),
			Phase:   string(pod.Status.Phase),
		})
	} else {
		_ = s.repo.UpdatePodStatus(string(pod.UID), string(pod.Status.Phase))
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleReportLink(c *gin.Context) {
	jobName := c.Param("jobName")

	var req struct {
		ReportUrl string `json:"report_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ReportUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "report_url is required"})
		return
	}

	if err := s.repo.SetJobReportUrl(jobName, req.ReportUrl); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parsedHook holds the platform-agnostic result of parsing an incoming webhook.
type parsedHook struct {
	pipeline     model.Pipeline
	trigger      string
	codeSha      string
	reportSha    string
	targetBranch string
	codeRef      string
	sourceUrl    string
	projectId    int
}

// parseWebhook parses a GitLab or Codeup webhook body and normalizes the
// fields the launcher and notifications need.
func parseWebhook(platform string, body io.ReadCloser, cb model.CodeBase, repoUrl string) (parsedHook, error) {
	var ph parsedHook
	switch platform {
	case "GitLab":
		p, err := gitlab.NewGitLabParser(body, cb.Url, cb.Token, cb.SkipTLSVerify)
		if err != nil {
			return ph, err
		}
		pipeline, err := p.Parse()
		if err != nil {
			return ph, fmt.Errorf("failed to parse pipeline: %w", err)
		}
		ph.pipeline = pipeline
		ph.trigger = p.Trigger
		ph.codeSha = p.CodeSha
		ph.reportSha = p.ReportSha
		ph.targetBranch = p.TargetBranch
		ph.codeRef = codeRefForTrigger(p.Trigger, p.Request.Ref)
		ph.projectId = p.Request.Project.Id
		ph.sourceUrl = parser.BuildSourceUrl("GitLab", p.Trigger, cb.Url, repoUrl, p.Request.Ref, p.CodeSha, p.Request.Attributes.Iid)
	case "Codeup":
		p, err := codeup.NewCodeupParser(body, cb.Url, cb.Token, cb.SkipTLSVerify)
		if err != nil {
			return ph, err
		}
		pipeline, err := p.Parse()
		if err != nil {
			return ph, fmt.Errorf("failed to parse pipeline: %w", err)
		}
		ph.pipeline = pipeline
		ph.trigger = p.Trigger
		ph.codeSha = p.CodeSha
		ph.reportSha = p.ReportSha
		ph.targetBranch = p.TargetBranch
		ph.codeRef = codeRefForTrigger(p.Trigger, p.Request.Ref)
		ph.projectId = p.Request.Project.Id
		if ph.projectId == 0 {
			ph.projectId = p.Request.ProjectId
		}
		if ph.projectId == 0 {
			ph.projectId = p.Request.Attributes.ProjectId
		}
		ph.sourceUrl = parser.BuildSourceUrl("Codeup", p.Trigger, cb.Url, repoUrl, p.Request.Ref, p.CodeSha, p.Request.Attributes.Iid)
	default:
		return ph, fmt.Errorf("unsupported platform: %s", platform)
	}
	return ph, nil
}

func (s *Server) handleWebhook(c *gin.Context) {
	id := c.Param("id")
	webhookConfig := s.repo.GetWebhookConfig(id)
	if webhookConfig.Id == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	platform := webhookConfig.WebhookType
	if _, ok := s.config.BaseConfig[platform]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s codebase not configured", platform)})
		return
	}

	ph, err := parseWebhook(platform, c.Request.Body, s.config.BaseConfig[platform], webhookConfig.RepoUrl)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var jobs []string
	for jobName, job := range ph.pipeline.Jobs {
		if !isValidTrigger(ph.trigger, job.Trigger) {
			continue
		}

		// resolve codebase config (pod override if available)
		baseCfg := s.config.BaseConfig[platform]
		if pod, ok := s.config.PodCodeBase[platform]; ok {
			baseCfg = pod
		}

		runnerConfig := model.RunnerConfig{
			CodebaseToken: baseCfg.Token,
			CodebaseUrl:   baseCfg.Url,
			ProjectId:     strconv.Itoa(ph.projectId),
			CommitSha:     ph.codeSha,
			ReportSha:     ph.reportSha,
			JobName:       jobName,
			Trigger:       ph.trigger,
			GitRepoUrl:    webhookConfig.RepoUrl,
			GitPrivateKey: "/etc/ssh/id_rsa",
			TargetBranch:  ph.targetBranch,
			CodeRef:       ph.codeRef,
			SourceUrl:     ph.sourceUrl,
		}

		// platform-specific extra env vars
		var extraEnv []v1.EnvVar
		extraEnv = append(extraEnv, v1.EnvVar{Name: "RUNNER_PLATFORM", Value: strings.ToLower(platform)})
		if baseCfg.SkipTLSVerify {
			extraEnv = append(extraEnv, v1.EnvVar{Name: "SKIP_TLS_VERIFY", Value: "true"})
		}
		if platform == "GitLab" && ph.targetBranch != "" {
			extraEnv = append(extraEnv, v1.EnvVar{Name: "TARGET_BRANCH", Value: ph.targetBranch})
		}
		// pass webhook URL query params as env vars to the pod
		for key, values := range c.Request.URL.Query() {
			extraEnv = append(extraEnv, v1.EnvVar{Name: key, Value: values[0]})
		}

		l := s.buildLauncher(runnerConfig, job.Image, job.Resources, platform, extraEnv)
		jobClient := s.clientSet.BatchV1().Jobs(s.config.Kubernetes.Namespace)
		createdJob, err := jobClient.Create(context.Background(), l.CreateJob(s.config.Host), metav1.CreateOptions{})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := s.repo.AddJob(internal.PipelineJob{
			ProjectId: id,
			Name:      createdJob.Name,
			Status:    "",
			Notify:    marshalNotify(job.Notify),
		}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		jobs = append(jobs, createdJob.Name)

		// Notify this job's targets: pipeline triggered
		statusUrl := fmt.Sprintf("%s/#/status/%s", s.config.Host, createdJob.Name)
		title := "🚀 流水线触发通知"
		content := fmt.Sprintf("📂 项目: %s\n📋 任务: %s\n🔄 触发: %s\n🔗 查看: %s", webhookConfig.RepoUrl, createdJob.Name, ph.trigger, statusUrl)
		if ph.sourceUrl != "" {
			content += fmt.Sprintf("\n📎 源码: %s", ph.sourceUrl)
		}
		s.sendJobNotifications(job.Notify, title, content)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "pipeline": ph.pipeline, "jobs": jobs})
}

func (s *Server) handleTrigger(c *gin.Context) {
	var req struct {
		RepoUrl string            `json:"repo_url"`
		JobName string            `json:"job_name"`
		Ref     string            `json:"ref"`
		Env     map[string]string `json:"env"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.RepoUrl == "" || req.JobName == "" || req.Ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repo_url, job_name, and ref are required"})
		return
	}

	// Find project by repo URL
	project := s.repo.GetProjectByRepoUrl(req.RepoUrl)
	if project.Id == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found for repo_url: " + req.RepoUrl})
		return
	}
	platform := project.WebhookType

	// Get platform config
	baseCfg, ok := s.config.BaseConfig[platform]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("platform %s not configured", platform)})
		return
	}
	podCfg := baseCfg
	if pod, ok := s.config.PodCodeBase[platform]; ok {
		podCfg = pod
	}

	// Fetch neutron.yaml from repo at given ref
	pipeline, err := parser.FetchPipeline(platform, req.RepoUrl, req.Ref, baseCfg.Url, baseCfg.Token, baseCfg.SkipTLSVerify)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to fetch pipeline: %v", err)})
		return
	}

	// Find the specified job
	job, ok := pipeline.Jobs[req.JobName]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("job '%s' not found in pipeline", req.JobName)})
		return
	}

	// Build runner config
	runnerConfig := model.RunnerConfig{
		CodebaseToken:      podCfg.Token,
		CodebaseUrl:        podCfg.Url,
		ProjectId:          "api",
		CommitSha:          req.Ref,
		ReportSha:          req.Ref,
		JobName:            req.JobName,
		Trigger:            "API",
		GitRepoUrl:         req.RepoUrl,
		GitPrivateKey:      "/etc/ssh/id_rsa",
		SkipTriggerCheck:   true,
		SkipPlatformReport: true,
		CodeRef:            codeRefForTrigger("API", req.Ref),
	}

	// Build extra env vars
	var extraEnv []v1.EnvVar
	extraEnv = append(extraEnv, v1.EnvVar{Name: "RUNNER_PLATFORM", Value: strings.ToLower(platform)})
	if podCfg.SkipTLSVerify {
		extraEnv = append(extraEnv, v1.EnvVar{Name: "SKIP_TLS_VERIFY", Value: "true"})
	}
	// Inject user-provided env vars
	for key, value := range req.Env {
		extraEnv = append(extraEnv, v1.EnvVar{Name: key, Value: value})
	}

	// Create K8s Job
	l := s.buildLauncher(runnerConfig, job.Image, job.Resources, platform, extraEnv)
	jobClient := s.clientSet.BatchV1().Jobs(s.config.Kubernetes.Namespace)
	createdJob, err := jobClient.Create(context.Background(), l.CreateJob(s.config.Host), metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create job: %v", err)})
		return
	}

	// Save job to database
	if err := s.repo.AddJob(internal.PipelineJob{
		ProjectId: project.Id,
		Name:      createdJob.Name,
		Status:    "",
		Notify:    marshalNotify(job.Notify),
	}); err != nil {
		log.Printf("failed to save job to database: %v", err)
	}

	// Send notifications
	statusUrl := fmt.Sprintf("%s/#/status/%s", s.config.Host, createdJob.Name)
	title := "🚀 流水线触发通知 (API)"
	content := fmt.Sprintf("📂 项目: %s\n📋 作业: %s\n🏷️ Ref: %s\n🔗 查看: %s", req.RepoUrl, req.JobName, req.Ref, statusUrl)
	s.sendJobNotifications(job.Notify, title, content)

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"job_name": createdJob.Name,
		"job_url":  statusUrl,
	})
}

// buildLauncher constructs a launcher with the K8s settings shared by the
// webhook and trigger flows.
func (s *Server) buildLauncher(rc model.RunnerConfig, image string, resources *model.Resources, platform string, extraEnv []v1.EnvVar) *launcher.Launcher {
	return launcher.NewLauncher(
		s.config.Kubernetes.Namespace,
		rc,
		s.config.Kubernetes.InitImage,
		s.config.Kubernetes.CheckoutImage,
		image,
		s.config.Kubernetes.GitPrivateKey,
		s.config.Kubernetes.ImagePullSecrets,
		platform,
		s.config.Kubernetes.PodApiUrl,
		resources,
		extraEnv...,
	)
}

func isValidTrigger(currentTrigger string, validTriggers []string) bool {
	for _, trigger := range validTriggers {
		if trigger == currentTrigger {
			return true
		}
	}
	return false
}

// codeRefForTrigger returns the CODE_REF value: tag name for TAG, branch name for PUSH, empty for MR.
func codeRefForTrigger(trigger, ref string) string {
	if trigger == "MR" {
		return ""
	}
	return parser.ExtractRefName(ref)
}

// marshalNotify serializes a job's notify config to JSON for persistence on
// neutron_job. A nil config yields an empty string.
func marshalNotify(n *model.Notify) string {
	if n == nil {
		return ""
	}
	b, err := json.Marshal(n)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseNotify deserializes the notify config persisted on neutron_job. An empty
// or invalid value yields nil (no notifications).
func parseNotify(s string) *model.Notify {
	if s == "" {
		return nil
	}
	var n model.Notify
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return nil
	}
	return &n
}
