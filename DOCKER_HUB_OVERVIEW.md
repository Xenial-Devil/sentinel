# Sentinel Docker Image - Full Detailed Overview

Sentinel is a safety-first Docker container update controller for self-hosted and server environments.

It runs beside your Docker daemon, detects when tracked container images have changed in a registry, and applies controlled updates with operational safeguards such as monitoring mode, approval gates, health validation, and rollback.

This image is built for operators who want update automation but still need production-grade control.

## Table of Contents

- What Sentinel Does
- How the Update Pipeline Works
- Image Build and Runtime Characteristics
- Quick Start
- Deployment Patterns
- Configuration Reference
- Private Registry Authentication
- Container Label Reference
- API Reference
- Metrics Reference
- Notifications and Webhooks
- Security and Hardening Guidance
- Data Persistence
- Operational Notes and Known Behavior
- Troubleshooting

## What Sentinel Does

Sentinel continuously watches selected containers and performs update cycles based on your policy.

Main capabilities:

- Scheduled update checks using interval polling or cron
- Digest-based update detection against registries
- Label-based watch targeting plus include/exclude container lists
- Approval workflow for controlled rollout
- Post-update health checks with automatic rollback
- Monitor-only mode for dry-run visibility
- Restart-only and no-restart operational modes
- Compose stack discovery and stack restart endpoints
- Built-in API and Prometheus metrics endpoints
- Slack, Teams, Email, and outbound webhook notifications

## How the Update Pipeline Works

For each cycle, Sentinel follows this flow:

1. Loads running containers from Docker (optionally including stopped/restarting containers).
2. Applies watch filters (exclude list, disable label, include list, scope, watch-all, watch labels).
3. Skips Sentinel itself by container name.
4. Runs pre-check hooks (global and per-container label hooks).
5. Evaluates mode:
   - Monitor-only: report and skip update action.
   - No-pull + no-restart: no action.
   - No-pull + restart: restart-only path.
   - Standard path: registry check and update path.
6. If approval mode is enabled, creates or waits for approval request before update.
7. Executes update:
   - Pull image
   - Stop container
   - Remove container
   - Recreate with preserved Docker config, host config, and networks
   - Start container
8. Runs health validation (if rollback enabled).
9. On health failure or recreate failure, attempts rollback to previous image/config state.
10. Emits notifications, webhook events, metrics, and post-check/update hooks.

## Image Build and Runtime Characteristics

The Docker image is built for minimal and predictable runtime behavior:

- Multi-stage build
- Builder: Go on Alpine
- Runtime: Alpine with CA certs, tzdata, and wget
- Binary: statically linked Go executable with CGO disabled
- Entrypoint: /usr/local/bin/sentinel
- Exposed ports: 8080 (API), 9090 (metrics)
- Working directory: /app
- Data directory: /app/data (create this as a mounted volume for persistence)

## Quick Start

### Docker Run

```bash
docker run -d \
  --name sentinel \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd)/data:/app/data \
  -e SENTINEL_API_ENABLED=true \
  -e SENTINEL_API_PORT=8080 \
  -e SENTINEL_METRICS_ENABLED=true \
  -e SENTINEL_METRICS_PORT=9090 \
  -e SENTINEL_WATCH_ALL=false \
  -e SENTINEL_LABEL_ENABLE=true \
  -e SENTINEL_WATCH_LABEL=com.sentinel.watch.enable \
  -e SENTINEL_WATCH_LABEL_VALUE=true \
  -e SENTINEL_APPROVAL_FILE=/app/data/approvals.json \
  -p 8080:8080 \
  -p 9090:9090 \
  isubroto/sentinel:latest
```

### Label In Your Target Services

```yaml
labels:
  com.sentinel.watch.enable: "true"
```

If SENTINEL_WATCH_ALL=true, labels are optional.

## Deployment Patterns

### Pattern 1: Label-Only Safe Rollout (Recommended)

- SENTINEL_WATCH_ALL=false
- SENTINEL_LABEL_ENABLE=true
- Add watch label only on services you want managed

### Pattern 2: Full Host Coverage

- SENTINEL_WATCH_ALL=true
- Use SENTINEL_EXCLUDE_CONTAINERS for critical exceptions

### Pattern 3: Staged Change Validation

- Start with SENTINEL_MONITOR_ONLY=true
- Observe logs, API, and metrics
- Switch to active mode after validation

### Pattern 4: Approval-Gated Production

- SENTINEL_APPROVAL=true
- Protect API with SENTINEL_API_TOKEN
- Approve or reject update requests through approval endpoints

## Configuration Reference

### Docker Connectivity

| Variable             | Default | Description                                                                                                      |
| -------------------- | ------- | ---------------------------------------------------------------------------------------------------------------- |
| SENTINEL_DOCKER_HOST | auto    | Docker endpoint. Linux default is unix:///var/run/docker.sock, Windows default is npipe:////./pipe/docker_engine |
| SENTINEL_TLS_VERIFY  | false   | Enable TLS verification for Docker daemon connection                                                             |
| SENTINEL_CERT_PATH   | empty   | Path containing ca.pem, cert.pem, and key.pem                                                                    |

### Scheduling and Target Selection

| Variable                    | Default                   | Description                                                            |
| --------------------------- | ------------------------- | ---------------------------------------------------------------------- |
| SENTINEL_POLL_INTERVAL      | 30                        | Interval in seconds for polling mode                                   |
| SENTINEL_CRON               | empty                     | Cron expression. If set, cron mode is used instead of interval polling |
| SENTINEL_WATCH_ALL          | false                     | Watch all eligible containers                                          |
| SENTINEL_LABEL_ENABLE       | true                      | Enable configured label watch matching                                 |
| SENTINEL_WATCH_LABEL        | com.sentinel.watch.enable | Label key used for opt-in watch                                        |
| SENTINEL_WATCH_LABEL_VALUE  | true                      | Required value for watch label                                         |
| SENTINEL_INCLUDE_CONTAINERS | empty                     | Comma-separated allow list by container name                           |
| SENTINEL_EXCLUDE_CONTAINERS | empty                     | Comma-separated deny list by container name                            |
| SENTINEL_SCOPE              | empty                     | Watch only containers whose sentinel.scope label matches this value    |
| SENTINEL_INCLUDE_STOPPED    | false                     | Include stopped containers in listing                                  |
| SENTINEL_INCLUDE_RESTARTING | false                     | Include restarting containers in listing                               |
| SENTINEL_REVIVE_STOPPED     | false                     | Start watched containers that are stopped/created                      |

### Update Behavior

| Variable                 | Default | Description                                         |
| ------------------------ | ------- | --------------------------------------------------- |
| SENTINEL_MONITOR_ONLY    | false   | Do not apply updates; only report/check             |
| SENTINEL_NO_PULL         | false   | Skip registry pull checks and pull operations       |
| SENTINEL_NO_RESTART      | false   | Pull only, do not restart container                 |
| SENTINEL_ROLLING_RESTART | false   | Use rolling-restart style stop/remove/recreate flow |
| SENTINEL_STOP_TIMEOUT    | 10      | Container stop timeout in seconds                   |
| SENTINEL_CLEANUP         | true    | Remove old image ID after successful update         |
| SENTINEL_REMOVE_VOLUMES  | false   | Remove anonymous volumes for updated containers     |
| SENTINEL_RUN_ONCE        | false   | Run a single cycle and exit                         |

### Safety and Governance

| Variable                | Default        | Description                                            |
| ----------------------- | -------------- | ------------------------------------------------------ |
| SENTINEL_ROLLBACK       | true           | Enable rollback on recreate failure or unhealthy state |
| SENTINEL_HEALTH_TIMEOUT | 30             | Seconds to wait for healthy status                     |
| SENTINEL_SEMVER_POLICY  | all            | Semver policy value (all, patch, minor, major, none)   |
| SENTINEL_APPROVAL       | false          | Require manual approval before applying updates        |
| SENTINEL_APPROVAL_FILE  | approvals.json | JSON file path used to persist approval requests       |

### Hooks

| Variable                    | Default | Description                         |
| --------------------------- | ------- | ----------------------------------- |
| SENTINEL_HOOK_PRE_CHECK     | empty   | Command run before update check     |
| SENTINEL_HOOK_POST_CHECK    | empty   | Command run after update check      |
| SENTINEL_HOOK_PRE_UPDATE    | empty   | Command run before update operation |
| SENTINEL_HOOK_POST_UPDATE   | empty   | Command run after update operation  |
| SENTINEL_HOOK_PRE_ROLLBACK  | empty   | Command run before rollback         |
| SENTINEL_HOOK_POST_ROLLBACK | empty   | Command run after rollback          |
| SENTINEL_HOOK_TIMEOUT       | 30      | Hook command timeout in seconds     |

### API, Metrics, and Logging

| Variable                 | Default | Description                           |
| ------------------------ | ------- | ------------------------------------- |
| SENTINEL_API_ENABLED     | true    | Enable HTTP API server                |
| SENTINEL_API_PORT        | 8080    | API listen port                       |
| SENTINEL_API_TOKEN       | empty   | Bearer token for protected API routes |
| SENTINEL_METRICS_ENABLED | true    | Enable metrics HTTP server            |
| SENTINEL_METRICS_PORT    | 9090    | Metrics listen port                   |
| SENTINEL_LOG_LEVEL       | info    | Log level                             |
| SENTINEL_LOG_FORMAT      | pretty  | Log format                            |

### Notifications and Webhooks

| Variable                         | Default            | Description                              |
| -------------------------------- | ------------------ | ---------------------------------------- |
| SENTINEL_SLACK_WEBHOOK           | empty              | Slack incoming webhook URL               |
| SENTINEL_TEAMS_WEBHOOK           | empty              | Microsoft Teams webhook URL              |
| SENTINEL_EMAIL_TO                | empty              | Recipient email for notifications        |
| SENTINEL_EMAIL_FROM              | sentinel@localhost | Sender email                             |
| SENTINEL_EMAIL_HOST              | smtp.gmail.com     | SMTP host                                |
| SENTINEL_EMAIL_PORT              | 587                | SMTP port                                |
| SENTINEL_EMAIL_USERNAME          | empty              | SMTP username                            |
| SENTINEL_EMAIL_PASSWORD          | empty              | SMTP password                            |
| SENTINEL_WEBHOOK_URL             | empty              | Outbound webhook URL                     |
| SENTINEL_WEBHOOK_SECRET          | empty              | Secret for HMAC SHA256 signature header  |
| SENTINEL_TEMPLATE_UPDATE_FOUND   | empty              | Custom template for update_found event   |
| SENTINEL_TEMPLATE_UPDATE_SUCCESS | empty              | Custom template for update_success event |
| SENTINEL_TEMPLATE_UPDATE_FAILED  | empty              | Custom template for update_failed event  |
| SENTINEL_TEMPLATE_ROLLBACK       | empty              | Custom template for rollback event       |
| SENTINEL_TEMPLATE_HEALTH_FAILED  | empty              | Custom template for health_failed event  |
| SENTINEL_TEMPLATE_STARTUP        | empty              | Custom template for startup event        |
| SENTINEL_NOTIFY_URL              | empty              | Legacy/reserved notification URL field   |

## Private Registry Authentication

Sentinel supports pulling images from private registries (e.g., ghcr.io, private Docker Hub, GitLab). Authentication credentials can be provided via environment variables or by mounting an existing Docker `config.json`.

Sentinel evaluates credentials in the following priority for each registry:
1. **Generic Credentials**: `REPO_USER` and `REPO_PASS` apply as a fallback to all registries.
2. **Per-Registry Override**: Set `SENTINEL_REGISTRY_USER_<HOST>` and `SENTINEL_REGISTRY_PASS_<HOST>` to isolate credentials per registry.
3. **Token-Only Override**: Set `SENTINEL_REGISTRY_TOKEN_<HOST>` for registries that support token-only access (like GitHub PATs).
4. **Docker Config**: Automatically loads credentials from the `DOCKER_CONFIG` directory if mounted (e.g., `/root/.docker`).

*Hostname Normalization*: For per-registry variables, convert the registry hostname to uppercase and replace dots (`.`), colons (`:`), and hyphens (`-`) with underscores (`_`). For example, `ghcr.io` becomes `GHCR_IO`.

## Container Label Reference

Sentinel supports per-container behavior through labels.

### Watch and Selection Labels

| Label                          | Purpose                                       |
| ------------------------------ | --------------------------------------------- |
| com.sentinel.watch.enable=true | Opt-in watch label (default configured label) |
| sentinel.enable=true           | Legacy opt-in watch label                     |
| sentinel.disable=true          | Force-disable watch for this container        |
| sentinel.scope=<value>         | Scope value used with SENTINEL_SCOPE          |

### Per-Container Mode Labels

| Label                         | Effect                          |
| ----------------------------- | ------------------------------- |
| sentinel.monitor-only=true    | Monitor-only for that container |
| sentinel.no-pull=true         | Skip pull and registry check    |
| sentinel.no-restart=true      | Pull-only update mode           |
| sentinel.rolling-restart=true | Use rolling restart path        |

### Per-Container Hook Labels

Label format:

- sentinel.hook.pre-check
- sentinel.hook.post-check
- sentinel.hook.pre-update
- sentinel.hook.post-update
- sentinel.hook.pre-rollback
- sentinel.hook.post-rollback

Hook command templates can use:

- {{container}}
- {{image}}

## API Reference

Sentinel API binds to SENTINEL_API_PORT (default 8080).

### Authentication

- If SENTINEL_API_TOKEN is empty, protected routes are open.
- If token is set, send Authorization: Bearer <token>.

### Public Endpoints

| Method | Path    | Description                             |
| ------ | ------- | --------------------------------------- |
| GET    | /health | Basic health response                   |
| GET    | /info   | Runtime configuration and feature state |

### Protected Update and Approval Endpoints

| Method | Path                    | Description                      |
| ------ | ----------------------- | -------------------------------- |
| POST   | /update                 | Trigger full update cycle        |
| POST   | /update/{container}     | Trigger update for one container |
| GET    | /approvals              | List approval state and requests |
| POST   | /approvals/approve/{id} | Approve pending request          |
| POST   | /approvals/reject/{id}  | Reject pending request           |

### Protected Compose Stack Endpoints

| Method | Path                  | Description                                 |
| ------ | --------------------- | ------------------------------------------- |
| GET    | /stacks               | List detected compose projects/services     |
| GET    | /stacks/{name}        | Get one compose project summary             |
| POST   | /stacks/{name}/update | Restart all containers in a compose project |

### Example API Calls

```bash
curl -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" \
  http://localhost:8080/info

curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" \
  http://localhost:8080/update

curl -X POST -H "Authorization: Bearer ${SENTINEL_API_TOKEN}" \
  http://localhost:8080/approvals/approve/<approval_id>
```

## Metrics Reference

Sentinel metrics server binds to SENTINEL_METRICS_PORT (default 9090).

Endpoints:

- /metrics
- /health

Exported metrics:

- sentinel_updates_total
- sentinel_updates_failed_total
- sentinel_rollbacks_total
- sentinel_pulls_total
- sentinel_pulls_failed_total
- sentinel_containers_watched
- sentinel_updates_pending
- sentinel_update_duration_seconds
- sentinel_pull_duration_seconds

## Notifications and Webhooks

### Channel Notifications

Sentinel can emit notifications to:

- Slack
- Microsoft Teams
- SMTP Email

Template variables available in custom templates:

- Event
- Timestamp
- Container
- Image
- OldImage
- NewImage
- Error
- Icon

### Outbound Webhooks

When SENTINEL_WEBHOOK_URL is configured, Sentinel sends signed JSON payloads (if secret set).

- Signature header: X-Sentinel-Signature
- Signature format: sha256=<hmac>

Webhook event types currently used by update flow include:

- pull.started
- pull.failed
- recreate.success
- rollback.done
- approval.pending

Payload fields include event, timestamp, container_name, image, old_image, new_image, error, and meta.

## Security and Hardening Guidance

Sentinel requires Docker daemon control access, so treat it as a privileged operator component.

Recommended hardening:

1. Restrict Docker socket exposure to Sentinel only.
2. Set SENTINEL_API_TOKEN and isolate API port to trusted networks.
3. Keep API and metrics behind reverse proxy/firewall in shared environments.
4. Use read-only root filesystem where possible and mount only required volumes.
5. Run with no-new-privileges and restart policy unless-stopped.
6. Monitor API access logs and webhook destinations.

## Data Persistence

Persist approval state by mapping a writable data volume and setting:

- SENTINEL_APPROVAL_FILE=/app/data/approvals.json

If approval persistence is not configured, approval state can be lost across container recreation.

## Operational Notes and Known Behavior

- Sentinel performs an immediate cycle on startup, then continues per schedule.
- If cron expression is invalid, Sentinel falls back to interval scheduling.
- Compose stack update endpoint performs container restarts for detected stack services.
- Hook commands run in Sentinel container context using shell execution.
- If API token is unset, protected routes allow unauthenticated access.
- Private registries are fully supported via Basic Auth and Bearer Token (WWW-Authenticate) challenge flows.

## Troubleshooting

### Sentinel does not watch expected containers

- Check SENTINEL_WATCH_ALL and label strategy.
- Verify include/exclude container name lists.
- Verify sentinel.disable is not set.
- Verify scope matching between SENTINEL_SCOPE and sentinel.scope.

### Updates are detected but not applied

- Check SENTINEL_MONITOR_ONLY.
- Check approval queue status at /approvals.
- Check container labels sentinel.no-pull or sentinel.no-restart.

### Rollback does not happen

- Verify SENTINEL_ROLLBACK=true.
- Verify target service defines a healthcheck if you rely on health-based rollback.

### API returns unauthorized

- Send Authorization Bearer token.
- Confirm token value matches SENTINEL_API_TOKEN exactly.

### No metrics visible

- Verify SENTINEL_METRICS_ENABLED=true.
- Check port mapping for SENTINEL_METRICS_PORT.
- Open /metrics endpoint directly.

Sentinel provides a practical balance between automation speed and production safety. For teams managing many containers, it centralizes update control, policy governance, and observability in a single lightweight image.
