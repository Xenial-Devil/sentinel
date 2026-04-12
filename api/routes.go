package api

import (
	"encoding/json"
	"net/http"
	"sentinel/approval"
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

// approvalManager is used for approval endpoints
var approvalManager *approval.Manager

// InitApproval sets up the approval manager
func InitApproval(cfg *config.Config) {
	approvalManager = approval.New(cfg.ApprovalFilePath)
}

// setupRoutes registers all API routes
func (a *API) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", a.handleHealth)

	// Info endpoint
	mux.HandleFunc("/info", a.handleInfo)

	// Trigger update cycle
	mux.HandleFunc("/update", Chain(
		a.handleUpdate,
		AuthMiddleware(a.Config.APIToken),
	))

	// Trigger specific container update
	mux.HandleFunc("/update/", Chain(
		a.handleUpdateContainer,
		AuthMiddleware(a.Config.APIToken),
	))

	// Approval endpoints
	mux.HandleFunc("/approvals", Chain(
		a.handleGetApprovals,
		AuthMiddleware(a.Config.APIToken),
	))

	mux.HandleFunc("/approvals/approve/", Chain(
		a.handleApprove,
		AuthMiddleware(a.Config.APIToken),
	))

	mux.HandleFunc("/approvals/reject/", Chain(
		a.handleReject,
		AuthMiddleware(a.Config.APIToken),
	))
	// Compose endpoints
	mux.HandleFunc("/stacks", Chain(
		a.handleGetStacks,
		AuthMiddleware(a.Config.APIToken),
	))
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

// handleInfo returns sentinel info
func (a *API) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Sentinel info",
		Data: map[string]interface{}{
			"version":       "1.0.0",
			"poll_interval": a.Config.PollInterval,
			"monitor_only":  a.Config.MonitorOnly,
			"watch_all":     a.Config.WatchAll,
		},
		Timestamp: time.Now(),
	})
}

// handleUpdate triggers a full update cycle
func (a *API) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success:   false,
			Message:   "Method not allowed",
			Timestamp: time.Now(),
		})
		return
	}

	logger.Log.Info("API triggered update cycle")
	go a.Watcher.RunCycle()

	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Update cycle triggered",
		Timestamp: time.Now(),
	})
}

// handleUpdateContainer triggers update for specific container
func (a *API) handleUpdateContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success:   false,
			Message:   "Method not allowed",
			Timestamp: time.Now(),
		})
		return
	}

	containerName := r.URL.Path[len("/update/"):]
	if containerName == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   "Container name required",
			Timestamp: time.Now(),
		})
		return
	}

	logger.Log.Infof("API triggered update for: %s", containerName)
	go a.Watcher.RunCycle()

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Update triggered for " + containerName,
		Data: map[string]string{
			"container": containerName,
		},
		Timestamp: time.Now(),
	})
}

// handleGetApprovals returns all approval requests
func (a *API) handleGetApprovals(w http.ResponseWriter, r *http.Request) {
	if approvalManager == nil {
		writeJSON(w, http.StatusOK, Response{
			Success:   true,
			Message:   "Approval not enabled",
			Timestamp: time.Now(),
		})
		return
	}

	pending := approvalManager.GetAll()

	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Approval requests",
		Data:      pending,
		Timestamp: time.Now(),
	})
}

// handleApprove approves a pending request
func (a *API) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success:   false,
			Message:   "Method not allowed",
			Timestamp: time.Now(),
		})
		return
	}

	// Get ID from URL
	id := r.URL.Path[len("/approvals/approve/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   "Approval ID required",
			Timestamp: time.Now(),
		})
		return
	}

	if approvalManager == nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   "Approval not enabled",
			Timestamp: time.Now(),
		})
		return
	}

	err := approvalManager.Approve(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Approved: " + id,
		Timestamp: time.Now(),
	})
}

// handleReject rejects a pending request
func (a *API) handleReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{
			Success:   false,
			Message:   "Method not allowed",
			Timestamp: time.Now(),
		})
		return
	}

	// Get ID from URL
	id := r.URL.Path[len("/approvals/reject/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   "Approval ID required",
			Timestamp: time.Now(),
		})
		return
	}

	if approvalManager == nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   "Approval not enabled",
			Timestamp: time.Now(),
		})
		return
	}

	err := approvalManager.Reject(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Rejected: " + id,
		Timestamp: time.Now(),
	})
}

// handleGetStacks returns all compose stacks
func (a *API) handleGetStacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{
		Success:   true,
		Message:   "Compose stacks",
		Timestamp: time.Now(),
	})
}
// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Log.Errorf("Failed to write JSON response: %v", err)
	}
	
}