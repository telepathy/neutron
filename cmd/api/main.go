package main

import (
	"context"
	"embed"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
	"html/template"
	"io/fs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"neutron/internal"
	"neutron/internal/gitlab"
	"neutron/internal/model"
	"neutron/internal/service"
	"os"
	"strconv"
	"time"
)

//go:embed files/*
var runnerBinFs embed.FS

//go:embed static/*
var staticFs embed.FS

//go:embed templates/*
var htmlFs embed.FS

func main() {
	var config model.Config
	data, _ := os.ReadFile("./config.yaml")
	_ = yaml.Unmarshal(data, &config)
	repo := internal.NewRepository(config)

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", config.Kubernetes.KubeConfig)
	if err != nil {
		panic(err)
	}
	clientSet, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	staticFs, err := fs.Sub(staticFs, "static")
	r.StaticFS("/static", http.FS(staticFs))
	tmpl := template.Must(template.ParseFS(htmlFs, "templates/*"))
	r.SetHTMLTemplate(tmpl)

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Message": "Hello!",
		})
	})

	r.GET("/loot", func(c *gin.Context) {
		looter := service.NewLooter(config.Kubernetes.Namespace, repo, config.Kubernetes.KubeConfig)
		err := looter.FetchCompletedJobLog()
		if err != nil {
			c.JSON(http.StatusInternalServerError, err.Error())
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		}
	})

	r.GET("/log/:podName", func(c *gin.Context) {
		podName := c.Param("podName")
		log, _ := repo.GetLogs(podName)
		if log != "" {
			c.HTML(http.StatusOK, "log_solid.html", gin.H{
				"PodName": podName,
				"Log":     log,
			})
		} else {
			c.HTML(http.StatusOK, "log.html", gin.H{
				"PodName": c.Param("podName"),
			})
		}

	})

	r.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhookConfig := repo.GetWebhookConfig(id)
		var pipeline model.Pipeline
		var err error
		var jobs []string
		switch webhookConfig.WebhookType {
		case "GitLab":
			p := gitlab.NewGitLabParser(c.Request.Body, config.BaseConfig["GitLab"].Url, config.BaseConfig["GitLab"].Token)
			pipeline, err = p.Parse()
			for jobName, job := range pipeline.Jobs {
				if !isValidTrigger(p.Trigger, job.Trigger) {
					continue
				}
				runnerConfig := gitlab.RunnerConfig{
					GitlabToken:   config.BaseConfig["GitLab"].Token,
					GitlabUrl:     config.BaseConfig["GitLab"].Url,
					ProjectId:     strconv.Itoa(p.Request.Project.Id),
					CommitSha:     p.CodeSha,
					ReportSha:     p.ReportSha,
					JobName:       jobName,
					Trigger:       p.Trigger,
					GitRepoUrl:    webhookConfig.RepoUrl,
					GitPrivateKey: "/etc/ssh/id_rsa",
				}
				l := gitlab.NewGitLabLauncher(
					config.Kubernetes.KubeConfig,
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

	r.GET("/runner-bin/:type", func(c *gin.Context) {
		runnerBinFile := fmt.Sprintf("files/neutron-%s-runner", c.Param("type"))
		file, err := runnerBinFs.Open(runnerBinFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		defer file.Close()

		fileContent, err := runnerBinFs.ReadFile(runnerBinFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		c.Header("Content-Disposition", "attachment; filename=\"runner\"")
		c.Data(http.StatusOK, "application/octet-stream", fileContent)
	})

	r.GET("/status/:jobName", func(c *gin.Context) {
		jobName := c.Param("jobName")
		status, err := repo.GetJobStatus(jobName)
		if err == nil {
			pods, _ := repo.GetPodStatus(jobName)
			c.HTML(http.StatusOK, "status_solid.html", gin.H{"jobName": jobName, "status": status, "pods": pods})
		} else {
			jobClient := clientSet.BatchV1().Jobs(config.Kubernetes.Namespace)
			job, err := jobClient.Get(context.Background(), jobName, metav1.GetOptions{})
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			}
			c.HTML(http.StatusOK, "status.html", gin.H{"job": job, "pods": pods})
		}
	})

	r.GET("/ws/logs/:podName", func(c *gin.Context) {
		podName := c.Param("podName")
		u := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := u.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		defer conn.Close()
		logStream, err := clientSet.CoreV1().Pods(config.Kubernetes.Namespace).GetLogs(podName, &corev1.PodLogOptions{Follow: true}).Stream(context.TODO())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		defer logStream.Close()
		buffer := make([]byte, 1024)
		for {
			n, err := logStream.Read(buffer)
			if err != nil {
				break
			}
			if err := conn.WriteMessage(websocket.TextMessage, buffer[:n]); err != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	_ = r.Run(fmt.Sprintf(":%d", config.Port))

}

func isValidTrigger(currentTrigger string, validTriggers []string) bool {
	for _, trigger := range validTriggers {
		if trigger == currentTrigger {
			return true
		}
	}
	return false
}
