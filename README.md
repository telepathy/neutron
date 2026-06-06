# Neutron - a lightweight pipeline system based on Kubernetes

## Architecture overview

![Concept Arch](./cmd/api/static/arch.svg)

Neutron is a lightweight CI/CD pipeline system built on Kubernetes. It consists of a stateless API server and a MySQL database. Code hosting platforms like GitLab send pipeline requests to the API server via webhooks. Neutron parses these requests, reads pipeline definitions from a `neutron.yaml` file in the repository, and creates Kubernetes Jobs to execute the pipeline steps. After a Job completes, the runner reports status (pending/running/success/fail) back to the codebase as commit statuses.

## How it works

1. A code event (push, tag, or merge request) triggers a webhook to Neutron's `/webhook/:id` endpoint
2. The API server parses the webhook payload, determines the trigger type (MR / TAG / PUSH)
3. Neutron fetches `neutron.yaml` from the Git repository via the GitLab API
4. For each job whose `trigger` list matches the current trigger type, a Kubernetes Job is created
5. Each K8s Job has two init containers:
   - **checkout** — clones the repository using SSH
   - **init** — downloads the embedded runner binary from the API server
6. The main container runs the runner binary, which reads `neutron.yaml`, executes steps sequentially, and reports status to GitLab
7. The Looter (`/loot`) collects logs from completed pods into MySQL

## Prerequisites

- Go 1.23+
- MySQL
- Kubernetes cluster with kubectl access
- GitLab instance with API access token

## Quick start

### 1. Build

```bash
# Build the runner binary first (it gets embedded into the API server)
make gitlab

# Build the API server
make api
```

### 2. Configure

Create `config.yaml` in the project root:

```yaml
host: "http://your-neutron-host"
port: 8888
database: "user:password@tcp(127.0.0.1:3306)/neutron?charset=utf8mb4&parseTime=True&loc=Local"
salt: "your-random-salt"

codebase:
  GitLab:
    url: "https://gitlab.example.com"
    token: "your-gitlab-private-token"

kubernetes:
  kube-config: "/path/to/.kube/config"
  namespace: "default"
  git-private-key: "git-ssh-secret"   # K8s secret name containing SSH key for git clone
  init-image: "curlimages/curl:latest" # image used by init container to download runner binary
```

### 3. Initialize database

```bash
mysql -u user -p < dds.sql
```

### 4. Run

```bash
./bin/neutron-api
```

## Pipeline configuration

Add a `neutron.yaml` file to the root of your repository:

```yaml
jobs:
  build:
    image: node:18-alpine
    trigger:
      - MR
      - PUSH
    steps:
      - name: install
        cmd: npm install
      - name: build
        cmd: npm run build
      - name: test
        cmd: npm test

  deploy:
    image: alpine:latest
    trigger:
      - TAG
    steps:
      - name: deploy
        cmd: echo "deploying..."
```

### Fields

| Field | Description |
|-------|-------------|
| `jobs.<name>` | Job identifier, used in the K8s Job name |
| `image` | Docker image for the pipeline container |
| `trigger` | List of trigger types that activate this job: `MR`, `TAG`, `PUSH` |
| `steps[].name` | Step name, reported as commit status context |
| `steps[].cmd` | Shell command to execute (parsed with `shellquote`) |

Steps run sequentially. If a step fails, all subsequent steps are marked as failed and the process exits.

## Register a project

Open `http://your-neutron-host/register` in a browser, or use the API:

```bash
curl -X POST http://localhost:8888/register \
  -d "webhookType=GitLab" \
  -d "repoUrl=git@gitlab.example.com:group/project.git"
```

The response includes the webhook URL to configure in GitLab (Settings → Webhooks):

```
POST http://your-neutron-host/webhook/<uuid>
```

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Web UI — index page |
| GET | `/register` | Web UI — register form |
| POST | `/register` | Register a project, returns webhook URL |
| POST | `/webhook/:id` | Receive webhook, create K8s Jobs |
| GET | `/status/:jobName` | View job and pod status |
| GET | `/log/:podName` | View pod log (from DB or live via K8s API) |
| GET | `/ws/logs/:podName` | WebSocket live log streaming |
| GET | `/loot` | Collect logs from completed K8s jobs into MySQL |
| GET | `/runner-bin/:type` | Download the embedded runner binary |

## Database schema

Three tables (`dds.sql`):

- **project** — registered projects (`id`, `webhook_type`, `repo_url`)
- **job** — K8s job metadata (`id`, `project_id`, `name`, `status` as JSON)
- **log** — pod execution logs (`id`, `job_name`, `pod_name`, `status`, `content`)

## Project structure

```
cmd/
  api/              # API server (Gin framework)
    main.go
    files/          # embedded runner binary (go:embed)
    static/         # embedded CSS + architecture diagram
    templates/      # embedded HTML templates
  gitlab-runner/    # runner binary (runs inside K8s pods)
    main.go
    reporter.go     # reports status to GitLab commit statuses
internal/
  gitlab/
    parser.go       # webhook parsing + neutron.yaml fetching
    laucher.go      # K8s Job creation
  model/
    config.go       # application config struct
    pipeline.go     # Pipeline/Job/Step models + interfaces
  service/
    runner.go       # step execution engine
    looter.go       # log collection from completed pods
  repo.go           # MySQL data access layer
```
