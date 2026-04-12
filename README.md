# 🛡️ Sentinel - Safe Container Deployment Controller

![Sentinel Banner](https://img.shields.io/badge/Sentinel-1.0.0-blue?style=for-the-badge&logo=docker)
![Go Version](https://img.shields.io/badge/Go-1.26.2-blue?style=for-the-badge&logo=go)

**Sentinel** is a powerful, safe, and automated deployment controller for Docker containers. It monitors your running containers, detects updates in remote registries, and applies them based on your safety policies—all while preserving your container configurations.

---

## ✨ Key Features

- 🔍 **Automated Monitoring**: Regularly polls your Docker engine or uses a cron schedule to check for container updates.
- 🛡️ **Safe Updates**: Recreates containers with exact match settings (volumes, environment variables, networks, labels) to ensure zero configuration loss.
- ⚙️ **Semver Policies**: Control updates based on Semantic Versioning (Major, Minor, Patch).
- 🤝 **Approval Workflow**: Gatekeep updates with a manual approval system via API.
- 📉 **Prometheus Metrics**: High-quality observability with built-in Prometheus metrics.
- 📢 **Multi-Channel Notifications**: Stay informed via Slack, Email, and Webhooks.
- 🔄 **Automatic Rollbacks**: (Coming Soon / Implementation in progress) Reverts to previous stable states if health checks fail.
- 📂 **Docker Compose Support**: Track and manage stacks of containers.

---

## 🚀 Getting Started

### Prerequisites

- Docker installed and running
- Go 1.26.2+ (if building from source)

### Run with Docker Compose

The easiest way to run Sentinel is using the provided `docker-compose.yml`:

```yaml
services:
  sentinel:
    container_name: sentinel
    image: sentinel:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
    environment:
      - SENTINEL_API_ENABLED=true
      - SENTINEL_METRICS_ENABLED=true
      - SENTINEL_WATCH_ALL=true
    ports:
      - "8080:8080"
      - "9090:9090"
```

---

## 🔧 Configuration

Sentinel is configured entirely via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `SENTINEL_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket path or host |
| `SENTINEL_POLL_INTERVAL` | `30` | Check interval in seconds |
| `SENTINEL_CRON` | `""` | Optional cron schedule (e.g., `0 * * * *`) |
| `SENTINEL_WATCH_ALL` | `true` | Monitor all containers by default |
| `SENTINEL_MONITOR_ONLY` | `false` | Only log updates, don't apply them |
| `SENTINEL_SEMVER_POLICY` | `all` | Version policy (`all`, `patch`, `minor`, `major`) |
| `SENTINEL_APPROVAL` | `false` | Enable manual approval for updates |
| `SENTINEL_API_ENABLED` | `true` | Enable the HTTP API |
| `SENTINEL_API_PORT` | `8080` | Port for the HTTP API |
| `SENTINEL_API_TOKEN` | `""` | Auth token for API protection |
| `SENTINEL_METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `SENTINEL_METRICS_PORT` | `9090` | Port for Prometheus metrics |

---

## 📡 API Documentation

### System Endpoints
- `GET /health`: Returns 200 OK if service is healthy.
- `GET /info`: Returns version and configuration info.

### Update Management
- `POST /update`: Manually trigger a full check cycle.
- `POST /update/{name}`: Trigger an update check for a specific container.

### Approval Management
- `GET /approvals`: List all pending and historical approval requests.
- `POST /approvals/approve/{id}`: Approve a specific update.
- `POST /approvals/reject/{id}`: Reject a specific update.

---

## 📊 Monitoring & Metrics

Sentinel exposes Prometheus metrics on `:9090/metrics` by default.

Key metrics include:
- `sentinel_updates_total`: Total successful updates.
- `sentinel_updates_failed_total`: Total failed updates.
- `sentinel_containers_watched`: Count of monitored containers.
- `sentinel_updates_pending`: Count of updates waiting for manual approval.
- `sentinel_update_duration_seconds`: Time taken for update operations.

---

## 🛠️ Architecture

Sentinel is built with a modular architecture:
- **Watcher**: Orchestrates the check cycles.
- **Registry**: Handles communication with remote container registries (Docker Hub, GHCR, etc.).
- **Updater**: Manages the Docker lifecycle (Inspect -> Pull -> Stop -> Remove -> Create -> Start).
- **Approval**: In-memory and file-based persistence for gated deployments.
- **Notifier**: Extensible notification dispatcher.

---

## 👩‍💻 Development Guide

### Prerequisites
- **Go**: 1.26.2 or higher
- **Docker**: Engine running (local or remote)
- **Git**: For cloning and dependency management

### Local Build
To build the binary locally:
```bash
go build -o sentinel .
```

To build the Docker image:
```bash
docker build -t sentinel:local .
```

### Project Structure
| Package | Description |
|---------|-------------|
| `api` | HTTP routes, handlers, and auth middleware. |
| `approval` | Management of pending/approved update requests. |
| `config` | Environment variable parsing and defaults. |
| `docker` | Wrapper for the official Docker SDK. |
| `logger` | Logrus-based filtered logging. |
| `metrics` | Prometheus metrics definitions and server. |
| `notifier` | Logic for Slack, Email, and Webhook alerts. |
| `registry` | Registry API interactions and digest comparisons. |
| `scheduler` | Tick-based and Cron-based execution logic. |
| `updater` | Implementation of the safe container swap process. |
| `watcher` | The main control loop that ties everything together. |

### Running Tests
Run the full test suite with:
```bash
go test ./...
```

### Local Development Setup
1. Clone the repository.
2. Ensure Docker is running.
3. Set up a `.env` file or export environment variables (see [Configuration](#-configuration)).
4. Run with `go run main.go`.

---

## 📄 License

This project is licensed under the MIT License.
