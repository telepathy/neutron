package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"neutron/internal"
	"neutron/internal/ccwork"
	"neutron/internal/notify"
)

//go:embed static/*
var staticFs embed.FS

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	repo := internal.NewRepository(config)

	// Initialize notify client
	var notifyClient *notify.Client
	if config.Notify.Url != "" && config.Notify.CorpId != "" && config.Notify.AppId != "" {
		notifyClient = notify.NewClient(config.Notify.Url, config.Notify.CorpId, config.Notify.AppId, config.Notify.SkipTLSVerify)
	}

	// Initialize CCWork robot client
	ccworkRobot := ccwork.NewRobot(config.Notify.SkipTLSVerify)

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

	server := NewServer(config, repo, clientSet, notifyClient, ccworkRobot)
	server.registerRoutes(r)

	// --- Snippet management ---

	r.GET("/api/snippets", func(c *gin.Context) {
		var projectId *int64
		if pidStr := c.Query("project_id"); pidStr != "" {
			if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
				projectId = &pid
			}
		}
		snippets, err := repo.ListSnippets(projectId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"snippets": snippets})
	})

	r.POST("/api/snippets", func(c *gin.Context) {
		var req struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Content     string `json:"content"`
			Description string `json:"description"`
			ProjectId   *int64 `json:"project_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Name == "" || req.Title == "" || req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name, title, and content are required"})
			return
		}
		// Check duplicate
		if _, err := repo.GetSnippetByName(req.Name); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "snippet name already exists"})
			return
		}
		snippet := internal.Snippet{
			Name:        req.Name,
			Title:       req.Title,
			Content:     req.Content,
			Description: req.Description,
			ProjectId:   req.ProjectId,
		}
		if err := repo.CreateSnippet(snippet); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "name": req.Name})
	})

	r.GET("/api/snippets/:name", func(c *gin.Context) {
		name := c.Param("name")
		snippet, err := repo.GetSnippetByName(name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "snippet not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"snippet": snippet})
	})

	r.PUT("/api/snippets/:name", func(c *gin.Context) {
		name := c.Param("name")
		if _, err := repo.GetSnippetByName(name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "snippet not found"})
			return
		}
		var req map[string]interface{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates := make(map[string]interface{})
		if v, ok := req["title"]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				updates["title"] = s
			}
		}
		if v, ok := req["content"]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				updates["content"] = s
			}
		}
		if v, ok := req["description"]; ok {
			updates["description"] = v
		}
		if v, ok := req["project_id"]; ok {
			updates["project_id"] = v
		}
		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
			return
		}
		if err := repo.UpdateSnippet(name, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.DELETE("/api/snippets/:name", func(c *gin.Context) {
		name := c.Param("name")
		if _, err := repo.GetSnippetByName(name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "snippet not found"})
			return
		}
		if err := repo.DeleteSnippet(name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Raw snippet endpoint for curl | bash / source <(curl)
	r.GET("/s/:name", func(c *gin.Context) {
		name := c.Param("name")
		snippet, err := repo.GetSnippetByName(name)
		if err != nil {
			c.String(http.StatusNotFound, "snippet not found")
			return
		}
		// Prepend query parameters as shell variable assignments
		var lines []string
		for key, values := range c.Request.URL.Query() {
			if len(values) > 0 {
				escaped := strings.ReplaceAll(values[0], `"`, `\"`)
				lines = append(lines, fmt.Sprintf(`%s="%s";`, key, escaped))
			}
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, snippet.Content)
		c.String(http.StatusOK, strings.Join(lines, "\n"))
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
