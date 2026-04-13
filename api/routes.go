package api

import (
	"encoding/json"
	"net/http"
	"sentinel/approval"
	"sentinel/compose"
	"sentinel/config"
	"sentinel/logger"
	"time"
)

// Response is the standard API response
type Response struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// InitApproval sets up the shared approval manager singleton
func InitApproval(cfg *config.Config) {
	approval.GetInstance(cfg.ApprovalFilePath)
}

func getApproval(cfg *config.Config) *approval.Manager {
	return approval.GetInstance(cfg.ApprovalFilePath)
}

// setupRoutes registers all API routes
func (a *API) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/info", a.handleInfo)

	// Update
	mux.HandleFunc("/update", Chain(a.handleUpdate, AuthMiddleware(a.Config.APIToken)))
	mux.HandleFunc("/update/", Chain(a.handleUpdateContainer, AuthMiddleware(a.Config.APIToken)))

	// Approvals
	mux.HandleFunc("/approvals", Chain(a.handleGetApprovals, AuthMiddleware(a.Config.APIToken)))
	mux.HandleFunc("/approvals/approve/", Chain(a.handleApprove, AuthMiddleware(a.Config.APIToken)))
	mux.HandleFunc("/approvals/reject/", Chain(a.handleReject, AuthMiddleware(a.Config.APIToken)))

	// Stacks - list, get, update
	mux.HandleFunc("/stacks", Chain(a.handleGetStacks, AuthMiddleware(a.Config.APIToken)))
	mux.HandleFunc("/stacks/", Chain(a.handleStackRouter, AuthMiddleware(a.Config.APIToken)))

	logger.Log.Debug("API routes registered")
	return mux
}

// handleHealth returns health status
func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Sentinel is healthy",
		Timestamp: time.Now(),
	})
}

// handleInfo returns full sentinel config info
func (a *API) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Sentinel info",
		Data: map[string]interface{}{
			"poll_interval":     a.Config.PollInterval,
			"monitor_only":      a.Config.MonitorOnly,
			"watch_all":         a.Config.WatchAll,
			"label_enable":      a.Config.LabelEnable,
			"watch_label":       a.Config.WatchLabel,
			"watch_label_value": a.Config.WatchLabelValue,
			"approval_enabled":  a.Config.ApprovalEnabled,
			"rollback_enabled":  a.Config.EnableRollback,
			"scope":             a.Config.Scope,
			"no_pull":           a.Config.NoPull,
			"no_restart":        a.Config.NoRestart,
			"rolling_restart":   a.Config.RollingRestart,
			"semver_policy":     a.Config.SemverPolicy,
			"cleanup":           a.Config.Cleanup,
			"stop_timeout":      a.Config.StopTimeout,
			"health_timeout":    a.Config.HealthTimeout,
			"run_once":          a.Config.RunOnce,
			"hooks": map[string]string{
				"pre_check":     a.Config.HookPreCheck,
				"post_check":    a.Config.HookPostCheck,
				"pre_update":    a.Config.HookPreUpdate,
				"post_update":   a.Config.HookPostUpdate,
				"pre_rollback":  a.Config.HookPreRollback,
				"post_rollback": a.Config.HookPostRollback,
			},
		},
		Timestamp: time.Now(),
	})
}

// handleUpdate triggers a full update cycle
func (a *API) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success: false, Message: "Method not allowed", Timestamp: time.Now(),
		})
		return
	}
	logger.Log.Info("API triggered full update cycle")
	go a.Watcher.RunCycle()
	writeJSON(w, http.StatusOK, Response{
		Success: true, Message: "Update cycle triggered", Timestamp: time.Now(),
	})
}

// handleUpdateContainer triggers update for a specific container
func (a *API) handleUpdateContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success: false, Message: "Method not allowed", Timestamp: time.Now(),
		})
		return
	}

	containerName := r.URL.Path[len("/update/"):]
	if containerName == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: "Container name required", Timestamp: time.Now(),
		})
		return
	}

	logger.Log.Infof("API triggered update for container: %s", containerName)

	go func() {
		if err := a.Watcher.RunContainerUpdate(containerName); err != nil {
			logger.Log.WithFields(logger.Fields{
				"container": containerName,
				"error":     err,
			}).Errorf("Container update failed: %v", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, Response{
		Success:   true,
		Message:   "Update triggered for " + containerName,
		Data:      map[string]string{"container": containerName},
		Timestamp: time.Now(),
	})
}

// handleGetApprovals returns all approval requests
func (a *API) handleGetApprovals(w http.ResponseWriter, r *http.Request) {
	if !a.Config.ApprovalEnabled {
		writeJSON(w, http.StatusServiceUnavailable, Response{
			Success: false, Message: "Approval mode not enabled", Timestamp: time.Now(),
		})
		return
	}
	mgr := getApproval(a.Config)
	all := mgr.GetAll()
	pending := mgr.GetPending()
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Approval requests",
		Data: map[string]interface{}{
			"total":   len(all),
			"pending": len(pending),
			"items":   all,
		},
		Timestamp: time.Now(),
	})
}

// handleApprove approves a pending request
func (a *API) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success: false, Message: "Method not allowed", Timestamp: time.Now(),
		})
		return
	}
	id := r.URL.Path[len("/approvals/approve/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: "Approval ID required", Timestamp: time.Now(),
		})
		return
	}
	if !a.Config.ApprovalEnabled {
		writeJSON(w, http.StatusServiceUnavailable, Response{
			Success: false, Message: "Approval mode not enabled", Timestamp: time.Now(),
		})
		return
	}
	if err := getApproval(a.Config).Approve(id); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: err.Error(), Timestamp: time.Now(),
		})
		return
	}
	logger.LogApprovalGranted("", id)
	writeJSON(w, http.StatusOK, Response{
		Success: true, Message: "Approved: " + id, Timestamp: time.Now(),
	})
}

// handleReject rejects a pending request
func (a *API) handleReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success: false, Message: "Method not allowed", Timestamp: time.Now(),
		})
		return
	}
	id := r.URL.Path[len("/approvals/reject/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: "Approval ID required", Timestamp: time.Now(),
		})
		return
	}
	if !a.Config.ApprovalEnabled {
		writeJSON(w, http.StatusServiceUnavailable, Response{
			Success: false, Message: "Approval mode not enabled", Timestamp: time.Now(),
		})
		return
	}
	if err := getApproval(a.Config).Reject(id); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: err.Error(), Timestamp: time.Now(),
		})
		return
	}
	logger.LogApprovalDenied("", id)
	writeJSON(w, http.StatusOK, Response{
		Success: true, Message: "Rejected: " + id, Timestamp: time.Now(),
	})
}

// handleGetStacks returns all compose stacks
func (a *API) handleGetStacks(w http.ResponseWriter, r *http.Request) {
	detector := compose.New(a.DockerClient.CLI)
	projects, err := detector.DetectProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{
			Success: false, Message: "Failed to detect stacks: " + err.Error(), Timestamp: time.Now(),
		})
		return
	}

	summaries := make(map[string]interface{})
	for name, project := range projects {
		summaries[name] = compose.GetProjectSummary(project)
	}

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Compose stacks",
		Data: map[string]interface{}{
			"total":  len(projects),
			"stacks": summaries,
		},
		Timestamp: time.Now(),
	})
}

// handleStackRouter routes /stacks/:name and /stacks/:name/update
func (a *API) handleStackRouter(w http.ResponseWriter, r *http.Request) {
	// path after /stacks/
	path := r.URL.Path[len("/stacks/"):]

	// Check if it's an update call: /stacks/:name/update
	if len(path) > 7 && path[len(path)-7:] == "/update" {
		projectName := path[:len(path)-7]
		if projectName == "" {
			writeJSON(w, http.StatusBadRequest, Response{
				Success: false, Message: "Stack name required", Timestamp: time.Now(),
			})
			return
		}
		a.handleUpdateStack(w, r, projectName)
		return
	}

	// Otherwise: /stacks/:name
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success: false, Message: "Stack name required", Timestamp: time.Now(),
		})
		return
	}
	a.handleGetStack(w, r, path)
}

// handleGetStack returns a single compose stack by name
func (a *API) handleGetStack(w http.ResponseWriter, r *http.Request, projectName string) {
	detector := compose.New(a.DockerClient.CLI)
	projects, err := detector.DetectProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{
			Success: false, Message: "Failed to detect stacks: " + err.Error(), Timestamp: time.Now(),
		})
		return
	}

	project, ok := projects[projectName]
	if !ok {
		writeJSON(w, http.StatusNotFound, Response{
			Success: false, Message: "Stack not found: " + projectName, Timestamp: time.Now(),
		})
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Stack: " + projectName,
		Data:      compose.GetProjectSummary(project),
		Timestamp: time.Now(),
	})
}

// handleUpdateStack restarts all containers in a compose stack
func (a *API) handleUpdateStack(w http.ResponseWriter, r *http.Request, projectName string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success: false, Message: "Method not allowed", Timestamp: time.Now(),
		})
		return
	}

	logger.Log.Infof("API triggered stack update: %s", projectName)

	detector := compose.New(a.DockerClient.CLI)
	projects, err := detector.DetectProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{
			Success: false, Message: "Failed to detect stacks: " + err.Error(), Timestamp: time.Now(),
		})
		return
	}

	project, ok := projects[projectName]
	if !ok {
		writeJSON(w, http.StatusNotFound, Response{
			Success: false, Message: "Stack not found: " + projectName, Timestamp: time.Now(),
		})
		return
	}

	// Run stack update in background
	go func() {
		updater := compose.NewStackUpdater(a.DockerClient.CLI, a.Config.StopTimeout)
		results := updater.UpdateProject(project)

		for _, result := range results {
			if result.Success {
				logger.Log.Infof("✅  Stack service updated: %s/%s",
					result.ProjectName, result.ServiceName)
			} else {
				logger.Log.Errorf("❌  Stack service failed: %s/%s - %v",
					result.ProjectName, result.ServiceName, result.Error)
			}
		}
	}()

	writeJSON(w, http.StatusAccepted, Response{
		Success: true,
		Message: "Stack update triggered for " + projectName,
		Data:    map[string]string{"stack": projectName},
		Timestamp: time.Now(),
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Log.Errorf("Failed to write JSON response: %v", err)
	}
}