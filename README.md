# 🛡️ Sentinel - Safe Container Deployment Controller

![Sentinel Banner](https://img.shields.io/github/v/release/Xenial-Devil/sentinel?display_name=tag&style=for-the-badge&logo=docker&label=Sentinel&color=blue)
![Go Version](https://img.shields.io/github/go-mod/go-version/Xenial-Devil/sentinel?style=for-the-badge&logo=go)

Sentinel is a safety-first Docker update controller.

It watches containers, detects image updates from registries, and applies controlled updates with safeguards such as approval gates, health validation, and rollback while preserving container runtime configuration.

---

## ✨ Feature Highlights

- Automated update checks using interval or cron scheduling
- Digest-based update detection for tracked images
- Safe recreate flow preserving container config, host config, and networks
- Manual approval workflow via API
- Health-check-driven rollback support
- Monitor-only mode for dry-run style operation
- Container-level behavior overrides through labels
- Prometheus metrics endpoint and health endpoints
- Slack, Teams, Email, and outbound webhook notifications
- Docker Compose project detection and stack-level restart endpoints

---

## 🧠 How Sentinel Works

For each cycle Sentinel:

1. Lists containers from Docker.
2. Filters containers by include/exclude/scope/watch rules.
3. Skips the Sentinel container itself.
4. Runs pre-check hooks (if configured).
5. Checks registry digest differences unless no-pull mode is enabled.
6. Optionally waits for approval in approval mode.
7. Pulls image, stops old container, removes it, recreates, and starts.
8. Waits for health status when rollback mode is enabled.
9. Rolls back on recreate/health failure.
10. Emits notifications, webhook events, and metrics.

---

## 🚀 Quick Start

### Run With Docker Compose (Recommended)

Use the repository [docker-compose.yml](docker-compose.yml):

```yaml
services:
  sentinel:
    container_name: sentinel
    image: isubroto/sentinel:latest
    restart: unless-stopped
    init: true
    env_file:
      - .env                         # ← registry credentials live here
    environment:
      SENTINEL_DOCKER_HOST: unix:///var/run/docker.sock
      SENTINEL_API_ENABLED: "true"
      SENTINEL_API_PORT: "8080"
      SENTINEL_METRICS_ENABLED: "true"
      SENTINEL_METRICS_PORT: "9090"
      SENTINEL_WATCH_ALL: "true"
      SENTINEL_LABEL_ENABLE: "true"
      SENTINEL_WATCH_LABEL: com.sentinel.watch.enable
      SENTINEL_WATCH_LABEL_VALUE: "true"
      SENTINEL_APPROVAL_FILE: /app/data/approvals.json
      SENTINEL_LOG_LEVEL: info
      SENTINEL_LOG_FORMAT: pretty
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
    ports:
      - "8080:8080"
      - "9090:9090"
```

```bash
# 1. Create your credentials file from the template
cp .env.example .env
# Fill in REPO_USER and REPO_PASS if using private registries (ghcr.io, etc.)

# 2. Start Sentinel
docker compose up -d
```


### Label-Based Opt-In Mode (Safer for Shared Hosts)

Set SENTINEL_WATCH_ALL=false and add a watch label only to containers you want managed:

```yaml
labels:
  com.sentinel.watch.enable: "true"
```

---

## 🔑 Private Registry Authentication

If your containers use images from **ghcr.io**, private Docker Hub repos, GitLab registry, or any other authenticated registry, Sentinel needs credentials to check for updates and pull new images.

### Why you see this error

```
private registry auth not configured for ghcr.io
```

This is the exact flow that triggers it:

```
getPrivateDigest("ghcr.io/org/app:latest")
  → anonymous fetch → 401 Unauthorized  (expected for private images)
  → GetCredentials("ghcr.io")  → nil     ← REPO_USER/REPO_PASS are empty
  → "private registry auth not configured for ghcr.io"  ❌
```

The fix is simply to provide credentials in your `.env` file.

### Step 1 – Generate a GitHub Personal Access Token (for ghcr.io)

```
GitHub → Settings → Developer Settings
  → Personal Access Tokens → Tokens (classic)
  → New Token

Required scopes:
  ✅ read:packages   (required – pull images)
  ✅ write:packages  (optional – only if Sentinel needs to push)
```

### Step 2 – Configure credentials (pick one option)

Copy `.env.example` to `.env` then fill in your values:

```bash
cp .env.example .env
```

**Option A — Generic (simplest, works for any registry)**

```env
REPO_USER=your_github_username
REPO_PASS=ghp_yourPersonalAccessToken
```

**Option B — Per-registry (recommended when you use multiple registries)**

```env
# ghcr.io
SENTINEL_REGISTRY_USER_GHCR_IO=your_github_username
SENTINEL_REGISTRY_PASS_GHCR_IO=ghp_yourPersonalAccessToken

# Docker Hub private
SENTINEL_REGISTRY_USER_REGISTRY_1_DOCKER_IO=dockerhub_user
SENTINEL_REGISTRY_PASS_REGISTRY_1_DOCKER_IO=dockerhub_token
```

**Option C — Token only (GitHub PAT without username)**

```env
SENTINEL_REGISTRY_TOKEN_GHCR_IO=ghp_yourPersonalAccessToken
```

**Option D — Mount existing Docker config.json**

```env
# Directory containing config.json (e.g. after docker login ghcr.io on the host)
DOCKER_CONFIG=/root/.docker
```

And mount it into the container:

```yaml
volumes:
  - /root/.docker:/root/.docker:ro
```

### Step 3 – Ensure your docker-compose.yml has env_file

```yaml
services:
  sentinel:
    image: isubroto/sentinel:latest
    env_file:
      - .env                         # ← loads REPO_USER, REPO_PASS, etc.
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data

  web:
    image: ghcr.io/isubroto/city_pos_fe:latest
    labels:
      com.sentinel.watch.enable: "true"
    restart: always

  app:
    image: ghcr.io/isubroto/city_pos_be:latest
    labels:
      com.sentinel.watch.enable: "true"
    restart: always
```

### Step 4 – Verify and restart

```bash
# Test your PAT manually first
echo "ghp_yourToken" | docker login ghcr.io -u your_username --password-stdin

# Restart Sentinel to pick up .env changes
docker compose down && docker compose up -d

# Watch logs – should now see update checks instead of auth errors
docker compose logs sentinel -f
```

### Credential priority order

Sentinel checks credentials in this order for each registry:

| Priority | Source | Example |
|---|---|---|
| 1st | `REPO_USER` + `REPO_PASS` | Generic, all registries |
| 2nd | `SENTINEL_REGISTRY_USER_<HOST>` + `SENTINEL_REGISTRY_PASS_<HOST>` | Per-registry override |
| 3rd | `SENTINEL_REGISTRY_TOKEN_<HOST>` | Token-only variant |
| 4th | `DOCKER_CONFIG` dir → `config.json` | Existing Docker login |

Hostname normalization for env var keys: uppercase, replace `.` `:` `-` with `_`

```
ghcr.io              → GHCR_IO
registry.k8s.io      → REGISTRY_K8S_IO
my-registry:5000     → MY_REGISTRY_5000
```

---


## 🎯 Use Case Playbooks

### 1. Dedicated host with fully automatic updates


Use this when the Docker host runs only services that are safe to auto-update.

```env
SENTINEL_WATCH_ALL=true
SENTINEL_APPROVAL=false
SENTINEL_ROLLBACK=true
```

### 2. Shared host with explicit opt-in control

Use this when the host has many containers and only selected ones should be managed.

```env
SENTINEL_WATCH_ALL=false
SENTINEL_LABEL_ENABLE=true
SENTINEL_WATCH_LABEL=com.sentinel.watch.enable
SENTINEL_WATCH_LABEL_VALUE=true
```

### 3. Production with human approval gates

Use this for critical workloads where every update needs operator approval.

```env
SENTINEL_APPROVAL=true
SENTINEL_API_ENABLED=true
SENTINEL_API_TOKEN=<strong-token>
SENTINEL_APPROVAL_FILE=/app/data/approvals.json
```

### 4. Security-only updates with patch policy

Use this when you want bugfix/security updates but want to avoid minor and major jumps.

```env
SENTINEL_SEMVER_POLICY=patch
SENTINEL_ROLLBACK=true
```

### 5. Dry-run validation before enabling updates

Use this to observe what Sentinel would update before allowing real changes.

```env
SENTINEL_MONITOR_ONLY=true
```

### 6. Pull image now, restart later

Use this to prefetch images during business hours and restart in a maintenance window.

```env
SENTINEL_NO_RESTART=true
```

### 7. Restart-only maintenance cycle

Use this to force container recycle without checking/pulling new images.

```env
SENTINEL_NO_PULL=true
SENTINEL_ROLLING_RESTART=true
```

### 8. Environment separation with scope labels

Use this when one Sentinel instance should only manage a specific scope like prod or staging.

```env
SENTINEL_SCOPE=prod
```

And on containers:

```yaml
labels:
  sentinel.scope: prod
```

### 9. One-shot execution in CI/CD jobs

Use this for scheduled pipelines that run Sentinel once and then exit.

```env
SENTINEL_RUN_ONCE=true
```

### 10. Recover watched services that were stopped

Use this for self-healing behavior on hosts where watched services should always be running.

```env
SENTINEL_INCLUDE_STOPPED=true
SENTINEL_REVIVE_STOPPED=true
```

---

## 🧩 Detailed Deployment Blueprints

### Blueprint A: Shared Host + Label Opt-In + Approval Gate

Use this when many unrelated workloads run on one host and updates must be approved.

```env
SENTINEL_WATCH_ALL=false
SENTINEL_LABEL_ENABLE=true
SENTINEL_WATCH_LABEL=com.sentinel.watch.enable
SENTINEL_WATCH_LABEL_VALUE=true
SENTINEL_APPROVAL=true
SENTINEL_API_TOKEN=<strong-token>
SENTINEL_APPROVAL_FILE=/app/data/approvals.json
SENTINEL_ROLLBACK=true
```

Example service label:

```yaml
labels:
  com.sentinel.watch.enable: "true"
```

Operational flow:

1. Sentinel detects an update and creates a pending approval.
2. Operator reviews pending items via API.
3. Operator approves/rejects each update.
4. Sentinel applies approved update and rolls back if health fails.

### Blueprint B: Patch-Only Production With Maintenance Window

Use this when production changes should happen only in a scheduled window and with low version-risk.

```env
SENTINEL_CRON=0 2 * * *
SENTINEL_SEMVER_POLICY=patch
SENTINEL_APPROVAL=true
SENTINEL_ROLLBACK=true
SENTINEL_HEALTH_TIMEOUT=90
```

Notes:

- Cron controls when cycles run.
- Patch policy avoids minor/major jumps.
- Higher health timeout helps slow-start services.

### Blueprint C: Prefetch During Day, Restart at Night

Use this when you want to reduce nighttime pull latency and control restart timing.

Daytime prefetch config:

```env
SENTINEL_NO_RESTART=true
```

Night maintenance actions:

1. Turn off no-restart mode.
2. Trigger targeted container update via API.
3. Validate health/metrics and then continue fleet rollout.

### Blueprint D: CI/CD One-Shot Verification Job

Use this for nightly or pre-release validation jobs where Sentinel runs once and exits.

```env
SENTINEL_RUN_ONCE=true
SENTINEL_MONITOR_ONLY=true
SENTINEL_WATCH_ALL=false
SENTINEL_INCLUDE_CONTAINERS=api,worker
```

Result:

- Pipeline receives deterministic one-cycle output.
- No changes are applied when monitor-only is enabled.

---

## ⚙️ Configuration Reference

Sentinel is configured fully with environment variables.

### Docker Connectivity

| Variable             | Default | Description                                                                                                  | Typical Use Case                                   |
| -------------------- | ------- | ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------- |
| SENTINEL_DOCKER_HOST | auto    | Docker endpoint. Linux default: unix:///var/run/docker.sock, Windows default: npipe:////./pipe/docker_engine | Override when connecting to a remote Docker daemon |
| SENTINEL_TLS_VERIFY  | false   | Enable TLS verification for Docker daemon connection                                                         | Required for secure remote daemon connections      |
| SENTINEL_CERT_PATH   | empty   | Path containing ca.pem, cert.pem, key.pem                                                                    | Set when Docker TLS client certs are used          |

### Scheduling and Selection

| Variable                    | Default                   | Description                                      | Typical Use Case                               |
| --------------------------- | ------------------------- | ------------------------------------------------ | ---------------------------------------------- |
| SENTINEL_POLL_INTERVAL      | 30                        | Poll interval (seconds)                          | Frequent update checks in always-on mode       |
| SENTINEL_CRON               | empty                     | Cron expression. If set, cron scheduling is used | Restrict checks to maintenance windows         |
| SENTINEL_WATCH_ALL          | false                     | Watch all containers                             | Dedicated host where all services are managed  |
| SENTINEL_LABEL_ENABLE       | true                      | Enable configured watch-label matching           | Shared host with explicit opt-in control       |
| SENTINEL_WATCH_LABEL        | com.sentinel.watch.enable | Watch label key                                  | Customize team-wide label convention           |
| SENTINEL_WATCH_LABEL_VALUE  | true                      | Required watch label value                       | Enforce consistent opt-in marker               |
| SENTINEL_INCLUDE_CONTAINERS | empty                     | Comma-separated allow list by container name     | Canary rollout to a shortlist of services      |
| SENTINEL_EXCLUDE_CONTAINERS | empty                     | Comma-separated deny list by container name      | Exclude databases or fragile legacy services   |
| SENTINEL_SCOPE              | empty                     | Match sentinel.scope container label             | Split management by environment or team        |
| SENTINEL_INCLUDE_STOPPED    | false                     | Include stopped containers                       | Audit stopped services during cycle checks     |
| SENTINEL_INCLUDE_RESTARTING | false                     | Include restarting containers                    | Observe unstable services during restart loops |
| SENTINEL_REVIVE_STOPPED     | false                     | Start watched stopped/created containers         | Self-healing for critical watched services     |

### Update Behavior

| Variable                 | Default | Description                                   | Typical Use Case                                 |
| ------------------------ | ------- | --------------------------------------------- | ------------------------------------------------ |
| SENTINEL_MONITOR_ONLY    | false   | Report/check only, do not update              | Dry-run mode before enabling automation          |
| SENTINEL_NO_PULL         | false   | Skip registry checks and image pulls          | Air-gapped host or restart-only workflows        |
| SENTINEL_NO_RESTART      | false   | Pull image only, do not restart container     | Pre-stage images before maintenance window       |
| SENTINEL_ROLLING_RESTART | false   | Use rolling restart stop/remove/recreate flow | Services that need controlled recycle behavior   |
| SENTINEL_STOP_TIMEOUT    | 10      | Container stop timeout (seconds)              | Increase for apps with long graceful shutdown    |
| SENTINEL_CLEANUP         | true    | Remove old image after successful update      | Save disk space on small hosts                   |
| SENTINEL_REMOVE_VOLUMES  | false   | Remove anonymous volumes on update            | Clean ephemeral workloads with throwaway volumes |
| SENTINEL_RUN_ONCE        | false   | Run one cycle and exit                        | CI/CD or scheduled one-shot execution            |

### Safety and Governance

| Variable                | Default        | Description                                   | Typical Use Case                                 |
| ----------------------- | -------------- | --------------------------------------------- | ------------------------------------------------ |
| SENTINEL_ROLLBACK       | true           | Enable rollback on failed recreate/health     | Production safety on critical workloads          |
| SENTINEL_HEALTH_TIMEOUT | 30             | Health wait timeout (seconds)                 | Increase for slow-starting services              |
| SENTINEL_SEMVER_POLICY  | all            | Semver policy: all, patch, minor, major, none | Limit updates to patch-only or minor-only tracks |
| SENTINEL_APPROVAL       | false          | Enable manual approval before update          | Change-managed environments                      |
| SENTINEL_APPROVAL_FILE  | approvals.json | Approval state file path                      | Persist approval history across restarts         |

### Hooks

| Variable                    | Default | Description                 | Typical Use Case                               |
| --------------------------- | ------- | --------------------------- | ---------------------------------------------- |
| SENTINEL_HOOK_PRE_CHECK     | empty   | Command run before check    | Validate prerequisites before each scan        |
| SENTINEL_HOOK_POST_CHECK    | empty   | Command run after check     | Emit custom audit logs after scan              |
| SENTINEL_HOOK_PRE_UPDATE    | empty   | Command run before update   | Drain traffic or disable alerts before restart |
| SENTINEL_HOOK_POST_UPDATE   | empty   | Command run after update    | Trigger smoke test or cache warm-up            |
| SENTINEL_HOOK_PRE_ROLLBACK  | empty   | Command run before rollback | Capture diagnostics before rollback action     |
| SENTINEL_HOOK_POST_ROLLBACK | empty   | Command run after rollback  | Notify incident systems after rollback         |
| SENTINEL_HOOK_TIMEOUT       | 30      | Hook timeout (seconds)      | Prevent stuck scripts from blocking cycles     |

### API, Metrics, Logging

| Variable                 | Default | Description                       | Typical Use Case                                 |
| ------------------------ | ------- | --------------------------------- | ------------------------------------------------ |
| SENTINEL_API_ENABLED     | true    | Enable API server                 | Integrate with automation and approval workflows |
| SENTINEL_API_PORT        | 8080    | API port                          | Move port to fit existing network policy         |
| SENTINEL_API_TOKEN       | empty   | Bearer token for protected routes | Secure API access in non-local environments      |
| SENTINEL_METRICS_ENABLED | true    | Enable metrics server             | Export Prometheus telemetry                      |
| SENTINEL_METRICS_PORT    | 9090    | Metrics port                      | Avoid conflicts with existing services           |
| SENTINEL_LOG_LEVEL       | info    | Log level                         | Set debug for troubleshooting                    |
| SENTINEL_LOG_FORMAT      | pretty  | Log format                        | Set json for centralized log ingestion           |

### Notifications and Webhooks

| Variable                         | Default            | Description                     | Typical Use Case                                     |
| -------------------------------- | ------------------ | ------------------------------- | ---------------------------------------------------- |
| SENTINEL_SLACK_WEBHOOK           | empty              | Slack incoming webhook URL      | Alert devops channels on update outcomes             |
| SENTINEL_TEAMS_WEBHOOK           | empty              | Teams webhook URL               | Notify operations teams using Teams                  |
| SENTINEL_EMAIL_TO                | empty              | Notification recipient          | Compliance-focused email reporting                   |
| SENTINEL_EMAIL_FROM              | sentinel@localhost | Notification sender             | Set sender identity for SMTP policies                |
| SENTINEL_EMAIL_HOST              | smtp.gmail.com     | SMTP host                       | Point to company SMTP relay                          |
| SENTINEL_EMAIL_PORT              | 587                | SMTP port                       | Match relay TLS submission settings                  |
| SENTINEL_EMAIL_USERNAME          | empty              | SMTP username                   | Authenticated SMTP environments                      |
| SENTINEL_EMAIL_PASSWORD          | empty              | SMTP password                   | Authenticated SMTP environments                      |
| SENTINEL_WEBHOOK_URL             | empty              | Outbound webhook URL            | Integrate with SIEM, incident, or automation systems |
| SENTINEL_WEBHOOK_SECRET          | empty              | HMAC signing secret             | Verify webhook authenticity on receiver side         |
| SENTINEL_TEMPLATE_UPDATE_FOUND   | empty              | Custom template: update_found   | Customize wording for update detected alerts         |
| SENTINEL_TEMPLATE_UPDATE_SUCCESS | empty              | Custom template: update_success | Customize successful rollout messages                |
| SENTINEL_TEMPLATE_UPDATE_FAILED  | empty              | Custom template: update_failed  | Add troubleshooting context in failure alerts        |
| SENTINEL_TEMPLATE_ROLLBACK       | empty              | Custom template: rollback       | Highlight rollback urgency and owner details         |
| SENTINEL_TEMPLATE_HEALTH_FAILED  | empty              | Custom template: health_failed  | Tailor health-failure escalation messages            |
| SENTINEL_TEMPLATE_STARTUP        | empty              | Custom template: startup        | Include host metadata on startup notifications       |
| SENTINEL_NOTIFY_URL              | empty              | Legacy/reserved field           | Reserved for backward compatibility                  |

---

## 🏷️ Container Labels

### Watch/Selection Labels

| Label                          | Effect                            | Typical Use Case                                     |
| ------------------------------ | --------------------------------- | ---------------------------------------------------- |
| com.sentinel.watch.enable=true | Opts container into watch set     | Shared host where only tagged services are managed   |
| sentinel.enable=true           | Legacy opt-in label               | Backward compatibility with older labeling           |
| sentinel.disable=true          | Forces Sentinel to skip container | Explicitly protect critical services from automation |
| sentinel.scope=<value>         | Assigns logical scope value       | Split updates by environment like prod/staging       |

### Per-Container Behavior Labels

| Label                         | Effect                                 | Typical Use Case                                  |
| ----------------------------- | -------------------------------------- | ------------------------------------------------- |
| sentinel.monitor-only=true    | Monitor-only for this container        | Observe one risky service before enabling updates |
| sentinel.no-pull=true         | Skip pull/check for this container     | Keep pinned images untouched while still tracked  |
| sentinel.no-restart=true      | Pull but do not restart this container | Preload image and restart manually later          |
| sentinel.rolling-restart=true | Force rolling restart behavior         | Enforce predictable recreate path per service     |

### Hook Labels

| Label                       | Trigger Point          | Typical Use Case                          |
| --------------------------- | ---------------------- | ----------------------------------------- |
| sentinel.hook.pre-check     | Before update check    | Validate external dependency availability |
| sentinel.hook.post-check    | After update check     | Emit audit events                         |
| sentinel.hook.pre-update    | Before update action   | Drain traffic from load balancer          |
| sentinel.hook.post-update   | After update action    | Trigger smoke test endpoint               |
| sentinel.hook.pre-rollback  | Before rollback action | Capture debug snapshot                    |
| sentinel.hook.post-rollback | After rollback action  | Open incident or notify on-call           |

Hook templates support:

- {{container}}
- {{image}}

---

## 📡 API Reference

When SENTINEL_API_TOKEN is set, protected routes require:

Authorization: Bearer <token>

If the token is empty, protected routes are open.

### Public Endpoints

| Method | Path    | Description                      | Typical Use Case                                        |
| ------ | ------- | -------------------------------- | ------------------------------------------------------- |
| GET    | /health | API health status                | Liveness checks from load balancers and uptime monitors |
| GET    | /info   | Runtime config and feature state | Validate active runtime settings during troubleshooting |

### Protected Endpoints

| Method | Path                    | Description                               | Typical Use Case                                    |
| ------ | ----------------------- | ----------------------------------------- | --------------------------------------------------- |
| POST   | /update                 | Trigger full check/update cycle           | Manual rollout trigger from CI/CD or ops dashboard  |
| POST   | /update/{container}     | Trigger update for one container          | Emergency update for a single vulnerable service    |
| GET    | /approvals              | List approval requests                    | Review pending updates during change-control window |
| POST   | /approvals/approve/{id} | Approve pending request                   | Authorize rollout after validation/sign-off         |
| POST   | /approvals/reject/{id}  | Reject pending request                    | Block risky or unplanned update                     |
| GET    | /stacks                 | List detected compose projects            | Discover compose projects managed on a host         |
| GET    | /stacks/{name}          | Get compose project summary               | Inspect service/container status for one stack      |
| POST   | /stacks/{name}/update   | Restart containers in the compose project | Planned maintenance recycle for an entire stack     |

Example:

```bash
curl -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" http://localhost:8080/info
curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" http://localhost:8080/update
```

### Approval Flow Walkthrough

1. List pending approvals:

```bash
curl -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" http://localhost:8080/approvals
```

2. Approve one request:

```bash
curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" \
  http://localhost:8080/approvals/approve/<approval_id>
```

3. Reject one request:

```bash
curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" \
  http://localhost:8080/approvals/reject/<approval_id>
```

4. Trigger a full cycle after decision:

```bash
curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" http://localhost:8080/update
```

---

## 📊 Metrics

Metrics server endpoint:

- /metrics
- /health

Exported Prometheus metrics:

| Metric                           | Meaning                            | Typical Use Case                 |
| -------------------------------- | ---------------------------------- | -------------------------------- |
| sentinel_updates_total           | Total successful updates           | Track rollout velocity over time |
| sentinel_updates_failed_total    | Total failed updates               | Alert on abnormal failure spikes |
| sentinel_rollbacks_total         | Total rollback actions             | Detect unstable image releases   |
| sentinel_pulls_total             | Total image pulls                  | Measure update-check throughput  |
| sentinel_pulls_failed_total      | Total failed pulls                 | Identify registry/network issues |
| sentinel_containers_watched      | Current watched container count    | Verify expected fleet coverage   |
| sentinel_updates_pending         | Pending approvals count            | Monitor approval queue backlog   |
| sentinel_update_duration_seconds | Update pipeline duration histogram | SLO tracking for update duration |
| sentinel_pull_duration_seconds   | Image pull duration histogram      | Diagnose slow registry pulls     |

### Health and Rollback Behavior Details

Sentinel health outcomes during update:

- healthy: update is considered successful.
- unhealthy: update fails and rollback is attempted when enabled.
- none: no healthcheck found, Sentinel treats container as healthy after short wait.

Best practice:

1. Define Docker HEALTHCHECK for every critical workload.
2. Keep SENTINEL_ROLLBACK=true in production.
3. Set SENTINEL_HEALTH_TIMEOUT high enough for cold start.

---

## 📢 Notifications and Outbound Webhooks

Notification channels:

- Slack
- Teams
- Email (SMTP)

Outbound webhook payloads can be signed using SENTINEL_WEBHOOK_SECRET.

- Signature header: X-Sentinel-Signature
- Signature format: sha256=<hmac>

Typical webhook events from runtime flow:

- pull.started
- pull.failed
- recreate.success
- rollback.done
- approval.pending

Example webhook payload:

```json
{
  "event": "approval.pending",
  "timestamp": "2026-04-13T10:15:00Z",
  "container_name": "payments-api",
  "image": "ghcr.io/acme/payments:1.4.2",
  "meta": {
    "host": "node-01",
    "version": "1.0.0"
  }
}
```

Receiver-side validation pattern:

1. Read request body as raw bytes.
2. Recompute HMAC SHA256 using SENTINEL_WEBHOOK_SECRET.
3. Compare with X-Sentinel-Signature.
4. Reject if signature mismatch.

---

## 🔐 Security Guidance

Sentinel needs Docker control-plane access. Treat it as a privileged operator component.

Recommended practices:

1. Restrict Docker socket access to trusted hosts/components.
2. Set SENTINEL_API_TOKEN and limit API exposure to trusted networks.
3. Keep metrics/API behind firewall or reverse proxy where needed.
4. Persist approval data in a mounted volume.
5. Use no-new-privileges and restart policies in container runtime.

---

## 👩‍💻 Development

### Prerequisites

- Go 1.26.2+
- Docker Engine
- Git

### Build and Run

```bash
go build -o sentinel .
go test ./...
go run main.go
```

### Build Container Image

```bash
docker build -t sentinel:local .
```

### Project Structure

| Package   | Description                                           |
| --------- | ----------------------------------------------------- |
| api       | HTTP routes, middleware, and handlers                 |
| approval  | Approval request lifecycle and persistence            |
| compose   | Compose project detection and stack updater           |
| config    | Environment variable parsing and defaults             |
| docker    | Docker client wrapper and container operations        |
| health    | Smoke and health helpers                              |
| hooks     | Lifecycle hook registration and execution             |
| logger    | Structured logging and formatting                     |
| metrics   | Prometheus metrics and server                         |
| notifier  | Slack, Teams, Email, and template-based notifications |
| registry  | Registry digest checks and auth helpers               |
| scheduler | Interval/cron cycle scheduler                         |
| updater   | Pull/recreate/rollback update pipeline                |
| watcher   | Main orchestration loop and filtering                 |
| webhook   | Outbound webhook event client                         |

---

## 📄 License

This project is licensed under the MIT License.
