# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Neutron is a lightweight CI/CD pipeline system built on Kubernetes. It receives webhooks from code hosting platforms (GitLab, Codeup), parses pipeline definitions from `neutron.yaml` in the repository, and launches Kubernetes Jobs to execute pipeline steps. Each runner reports status back to both the platform (GitLab commit statuses; Codeup no-op) and the Neutron API server for persistence and history tracking.

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
- `POST /webhook/:id` — receives webhooks, auto-detects platform (GitLab/Codeup) via `X-Codeup-Event` header, fetches `neutron.yaml`, creates K8s Jobs. Query params on the webhook URL are passed as env vars to the pod.
- `GET /api/projects` — lists all registered projects
- `GET /api/projects/:id/jobs` — lists jobs for a project (last 7 days)
- `GET /api/status/:jobName` — job/pod status (JSON, from DB for completed jobs or K8s API for active jobs)
- `POST /api/report/:jobName` — runners push status back to API server for persistence
- `GET/POST/DELETE /api/projects/:id/recipients` — manage IM notification recipients (per-user)
- `GET/POST/DELETE /api/projects/:id/ccwebhooks` — manage CCWork group webhook URLs (per-project)
- SPA: `cmd/api/static/index.html` — vanilla JS with hash-based routing (#/, #/projects, #/project/:id, #/status/:name)

**GitLab Runner** (`cmd/gitlab-runner/`) — runs inside K8s pods for GitLab projects:
- Reads config from environment variables (set by API server when creating the Job)
- Reads `neutron.yaml` from cloned repo, executes steps sequentially
- Uses CompositeReporter: reports to GitLab commit statuses + Neutron API (`/api/report/:jobName`)

**Codeup Runner** (`cmd/codeup-runner/`) — runs inside K8s pods for Codeup projects:
- Same execution logic as GitLab runner
- Uses CompositeReporter: NoOp reporter (Codeup has no status API) + Neutron API (`/api/report/:jobName`)

### Data Flow

1. Webhook → `/webhook/:id` (GitLab or Codeup)
2. API server auto-detects platform, parses webhook, fetches `neutron.yaml` via platform API
3. API server creates K8s Job with platform-appropriate runner binary
4. Main container runs runner → reads `neutron.yaml` → executes steps → reports status to both the platform API and Neutron API (`/api/report/:jobName`)
5. Status queries (`/api/status/:jobName`) return from DB for completed jobs, or K8s API + persist to DB for active jobs

### Key Packages

- `internal/gitlab/` — GitLab webhook parsing (`parser.go`)
- `internal/codeup/` — Codeup webhook parsing (`parser.go`)
- `internal/launcher/` — shared K8s Job creation (platform-agnostic)
- `internal/model/` — domain models: `Config`, `Pipeline`, `Job`, `Step`, `RunnerConfig` + interfaces: `Reporter`, `PipelineParser`
- `internal/service/` — `Runner` (step execution)
- `internal/repo.go` — MySQL data access (Repository pattern)
- `internal/notify/` — IM notification client (enterprise messaging, attachment format)
- `internal/ccwork/` — CCWork robot webhook client (group notifications, attachment format)
- `internal/reporter/` — Composite reporter (fan-out to multiple backends)
- `cmd/api/` — API server with embedded SPA (static/index.html)
- `cmd/gitlab-runner/` — GitLab runner binary + CompositeReporter (GitLab + Neutron)
- `cmd/codeup-runner/` — Codeup runner binary + CompositeReporter (NoOp + Neutron)

### Database (MySQL)

Tables auto-migrated by GORM:
- `neutron_project` (id, webhook_type, repo_url)
- `neutron_job` (id, project_id, name, status, completed, completed_at)
- `neutron_pod` (id, job_id, pod_name, pod_uid, phase)
- `neutron_notify` (id, project_id, user_id) — IM notification recipients per project
- `neutron_ccwebhook` (id, project_id, webhook_url, description) — CCWork group webhooks per project

### Configuration

Runtime config is `config.yaml` (gitignored). Shape defined by `internal/model/config.go`: host, port, database (MySQL DSN), salt, log_url (external log platform link template with {namespace} and {podName} placeholders, optional), codebase map (url/token/skip_tls_verify per platform: GitLab, Codeup), pod_codebase (pod-side codebase addresses, optional), kubernetes (kube-config path — optional for in-cluster, auto-detected via ServiceAccount; required for out-of-cluster, namespace, git-private-key secret, init-image, checkout-image, image-pull-secrets), notify (IM notification config: url, corp_id, app_id, skip_tls_verify). Most fields can be overridden via environment variables (NEUTRON_*).

### Notifications

Two notification channels, always sent in parallel on pipeline trigger and completion:
1. **IM notifications** (`internal/notify/`) — sends to individual users via enterprise IM bot API. Recipients managed per project in `neutron_notify` table.
2. **CCWork group webhooks** (`internal/ccwork/`) — sends to group chats via webhook URLs. Webhooks managed per project in `neutron_ccwebhook` table, each with a description label.

Both use structured attachment format with title (head) and body content.

### Webhook URL Parameters

Custom parameters can be passed to pipeline pods by appending query parameters to the webhook URL:

```
POST https://neutron.example.com/webhook/abc-123?DEPLOY_ENV=prod&IMAGE_TAG=v1.2.3
```

All query parameters are injected as environment variables into the K8s Job's main container. Step commands can reference them directly via `$DEPLOY_ENV`, `$IMAGE_TAG`, etc.

## Conventions

- Go 1.23.0, Go modules (no vendor)
- Module name: `neutron`
- No test suite or linting config exists yet
- `test.http` contains manual HTTP requests for JetBrains HTTP Client
