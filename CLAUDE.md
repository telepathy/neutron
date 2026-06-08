# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Neutron is a lightweight CI/CD pipeline system built on Kubernetes. It receives webhooks from code hosting platforms (GitLab, Codeup), parses pipeline definitions from `neutron.yaml` in the repository, and launches Kubernetes Jobs to execute pipeline steps. GitLab status is reported via commit statuses; Codeup has no status API (logged as TODO).

## Build Commands

```bash
make api      # builds API server → bin/neutron-api (CGO_ENABLED=0, statically linked)
make gitlab   # builds GitLab runner → bin/neutron-gitlab-runner
make codeup   # builds Codeup runner → bin/neutron-codeup-runner

# Full rebuild
make clean && make api && make gitlab && make codeup

# Build Docker images and load into kind
make kind-load

# Run tests (none exist yet)
go test ./...
```

## Architecture

### Three Binaries

**API Server** (`cmd/api/main.go`) — Gin-based HTTP server with SPA frontend:
- `GET /api/config` — returns runtime config (log URL template, namespace) for SPA
- `POST /api/register` — registers a project webhook (stores in MySQL with UUID)
- `POST /webhook/:id` — receives webhooks, auto-detects platform (GitLab/Codeup) via `X-Codeup-Event` header, fetches `neutron.yaml`, creates K8s Jobs
- `GET /api/status/:jobName` — job/pod status (JSON, from DB or K8s API)
- SPA: `cmd/api/static/index.html` — vanilla JS with hash-based routing (#/register, #/status/:name)

**GitLab Runner** (`cmd/gitlab-runner/`) — runs inside K8s pods for GitLab projects:
- Reads config from environment variables (set by API server when creating the Job)
- Reads `neutron.yaml` from cloned repo, executes steps sequentially
- Reports status (pending/running/success/fail) to GitLab commit statuses

**Codeup Runner** (`cmd/codeup-runner/`) — runs inside K8s pods for Codeup projects:
- Same execution logic as GitLab runner
- No-op reporter (Codeup has no pipeline status API) — logs TODO

### Data Flow

1. Webhook → `/webhook/:id` (GitLab or Codeup)
2. API server auto-detects platform, parses webhook, fetches `neutron.yaml` via platform API
3. API server creates K8s Job with platform-appropriate runner binary
4. Main container runs runner → reads `neutron.yaml` → executes steps → reports status (GitLab) or logs TODO (Codeup)

### Key Packages

- `internal/gitlab/` — GitLab webhook parsing (`parser.go`)
- `internal/codeup/` — Codeup webhook parsing (`parser.go`)
- `internal/launcher/` — shared K8s Job creation (platform-agnostic)
- `internal/model/` — domain models: `Config`, `Pipeline`, `Job`, `Step`, `RunnerConfig` + repository interfaces
- `internal/service/` — `Runner` (step execution)
- `internal/repo.go` — MySQL data access (Repository pattern)
- `cmd/api/` — API server with embedded SPA (static/index.html)
- `cmd/gitlab-runner/` — GitLab runner binary + GitLab reporter
- `cmd/codeup-runner/` — Codeup runner binary + no-op reporter

### Database (MySQL)

Two tables defined in `dds.sql`: `project` (id, webhook_type, repo_url), `job` (id, project_id, name, status JSON).

### Configuration

Runtime config is `config.yaml` (gitignored). Shape defined by `internal/model/config.go`: host, port, database (MySQL DSN), salt, log_url (external log platform link template with {namespace} and {podName} placeholders, optional), codebase map (url/token pairs per platform: GitLab, Codeup), pod_codebase (pod-side codebase addresses, optional), kubernetes (kube-config path — optional for in-cluster deployment, auto-detected via ServiceAccount; required for out-of-cluster, namespace, git-private-key secret, init-image, checkout-image).

## Conventions

- Go 1.23.0, vendored dependencies (`vendor/`)
- Module name: `neutron`
- No test suite or linting config exists yet
- `test.http` contains manual HTTP requests for JetBrains HTTP Client
