# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Neutron is a lightweight CI/CD pipeline system built on Kubernetes. It receives webhooks from code hosting platforms (currently GitLab), parses pipeline definitions from `neutron.yaml` in the repository, and launches Kubernetes Jobs to execute pipeline steps. Status is reported back to the codebase via commit statuses.

## Build Commands

```bash
make gitlab   # builds runner binary → bin/neutron-gitlab-runner
make api      # builds API server → bin/neutron-api (CGO_ENABLED=0, statically linked)

# Full rebuild
make clean && make gitlab && make api

# Build Docker images and load into kind
make kind-load

# Run tests (none exist yet)
go test ./...
```

## Architecture

### Two Binaries

**API Server** (`cmd/api/main.go`) — Gin-based HTTP server:
- `/register` — registers a project webhook (stores in MySQL with UUID)
- `/webhook/:id` — receives GitLab webhooks, fetches `neutron.yaml` from the repo, creates K8s Jobs
- `/ws/logs/:podName` — WebSocket live log streaming
- `/status/:jobName` — job/pod status view
- `/loot` — triggers log collection from completed K8s jobs into MySQL
- Web UI pages: index, register, status, logs (HTML templates in `cmd/api/templates/`)

**GitLab Runner** (`cmd/gitlab-runner/main.go`) — runs inside K8s pods:
- Reads config from environment variables (set by API server when creating the Job)
- Reads `neutron.yaml` from cloned repo, executes steps sequentially
- Reports status (pending/running/success/fail) to GitLab commit statuses

### Data Flow

1. GitLab webhook → `/webhook/:id`
2. API server parses webhook, fetches `neutron.yaml` via GitLab API
3. API server creates K8s Job (init containers: git-clone repo + copy runner binary from runner image)
4. Main container runs runner binary → reads `neutron.yaml` → executes steps → reports to GitLab
5. Looter (manual or cron) collects logs from completed pods into MySQL

### Key Packages

- `internal/gitlab/` — webhook parsing (`parser.go`) and K8s Job creation (`launcher.go`)
- `internal/model/` — domain models: `Config`, `Pipeline`, `Job`, `Step` + repository interfaces
- `internal/service/` — `Runner` (step execution) and `Looter` (log collection)
- `internal/repo.go` — MySQL data access (Repository pattern)
- `cmd/api/` — API server with embedded static files and templates
- `cmd/gitlab-runner/` — runner binary + reporter

### Database (MySQL)

Three tables defined in `dds.sql`: `project` (id, webhook_type, repo_url), `job` (id, project_id, name, status JSON), `log` (id, job_name, pod_name, status, content).

### Configuration

Runtime config is `config.yaml` (gitignored). Shape defined by `internal/model/config.go`: host, port, database (MySQL DSN), salt, codebase map (url/token pairs), pod_codebase (pod-side codebase addresses, optional), kubernetes (kube-config path, namespace, git-private-key secret, init-image).

## Conventions

- Go 1.23.0, vendored dependencies (`vendor/`)
- Module name: `neutron`
- No test suite or linting config exists yet
- `test.http` contains manual HTTP requests for JetBrains HTTP Client
