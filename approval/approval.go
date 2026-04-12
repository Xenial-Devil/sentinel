package approval

import (
	"encoding/json"
	"fmt"
	"os"
	"sentinel/logger"
	"sync"
	"time"
)

// Status represents approval status
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

// Request holds a pending approval request
type Request struct {
	ID            string    `json:"id"`
	ContainerName string    `json:"container_name"`
	CurrentImage  string    `json:"current_image"`
	NewImage      string    `json:"new_image"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ApprovedAt    time.Time `json:"approved_at,omitempty"`
	RejectedAt    time.Time `json:"rejected_at,omitempty"`
}

// Manager handles approval requests
type Manager struct {
	mu       sync.RWMutex
	requests map[string]*Request
	filePath string
}

// New creates a new approval Manager
func New(filePath string) *Manager {
	m := &Manager{
		requests: make(map[string]*Request),
		filePath: filePath,
	}

	// Load existing requests from file
	m.load()

	return m
}

// RequestApproval creates a new approval request
func (m *Manager) RequestApproval(
	containerName string,
	currentImage string,
	newImage string,
) *Request {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build unique ID
	id := buildID(containerName, newImage)

	// Check if already pending
	if existing, ok := m.requests[id]; ok {
		if existing.Status == StatusPending {
			logger.Log.Infof("Approval already pending for %s", containerName)
			return existing
		}
	}

	// Create new request
	req := &Request{
		ID:            id,
		ContainerName: containerName,
		CurrentImage:  currentImage,
		NewImage:      newImage,
		Status:        StatusPending,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	m.requests[id] = req

	// Save to file
	m.save()

	logger.Log.Infof("Approval requested for %s: %s -> %s",
		containerName,
		currentImage,
		newImage,
	)

	return req
}

// Approve approves a pending request
func (m *Manager) Approve(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return fmt.Errorf("approval request not found: %s", id)
	}

	if req.Status != StatusPending {
		return fmt.Errorf("request is not pending: %s", req.Status)
	}

	// Check if expired
	if time.Now().After(req.ExpiresAt) {
		req.Status = StatusExpired
		m.save()
		return fmt.Errorf("approval request has expired")
	}

	req.Status = StatusApproved
	req.ApprovedAt = time.Now()

	m.save()

	logger.Log.Infof("Approved update for %s: %s",
		req.ContainerName,
		req.NewImage,
	)

	return nil
}

// Reject rejects a pending request
func (m *Manager) Reject(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return fmt.Errorf("approval request not found: %s", id)
	}

	if req.Status != StatusPending {
		return fmt.Errorf("request is not pending: %s", req.Status)
	}

	req.Status = StatusRejected
	req.RejectedAt = time.Now()

	m.save()

	logger.Log.Infof("Rejected update for %s: %s",
		req.ContainerName,
		req.NewImage,
	)

	return nil
}

// IsApproved checks if an update is approved
func (m *Manager) IsApproved(containerName string, newImage string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id := buildID(containerName, newImage)

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	// Check expiry
	if time.Now().After(req.ExpiresAt) {
		logger.Log.Warnf("Approval expired for %s", containerName)
		return false
	}

	return req.Status == StatusApproved
}

// IsPending checks if an update is pending approval
func (m *Manager) IsPending(containerName string, newImage string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id := buildID(containerName, newImage)

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	return req.Status == StatusPending
}

// GetPending returns all pending requests
func (m *Manager) GetPending() []*Request {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []*Request

	for _, req := range m.requests {
		// Skip expired
		if time.Now().After(req.ExpiresAt) {
			req.Status = StatusExpired
			continue
		}

		if req.Status == StatusPending {
			pending = append(pending, req)
		}
	}

	return pending
}

// GetAll returns all requests
func (m *Manager) GetAll() []*Request {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*Request
	for _, req := range m.requests {
		all = append(all, req)
	}

	return all
}

// CleanExpired removes expired requests
func (m *Manager) CleanExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, req := range m.requests {
		if time.Now().After(req.ExpiresAt) {
			delete(m.requests, id)
			logger.Log.Debugf("Removed expired approval: %s", id)
		}
	}

	m.save()
}

// save saves requests to file
func (m *Manager) save() {
	if m.filePath == "" {
		return
	}

	data, err := json.MarshalIndent(m.requests, "", "  ")
	if err != nil {
		logger.Log.Errorf("Failed to marshal approvals: %v", err)
		return
	}

	err = os.WriteFile(m.filePath, data, 0644)
	if err != nil {
		logger.Log.Errorf("Failed to save approvals: %v", err)
	}
}

// load loads requests from file
func (m *Manager) load() {
	if m.filePath == "" {
		return
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		// File does not exist yet
		return
	}

	err = json.Unmarshal(data, &m.requests)
	if err != nil {
		logger.Log.Errorf("Failed to load approvals: %v", err)
	}

	logger.Log.Infof("Loaded %d approval requests", len(m.requests))
}

// buildID creates unique ID for a request
func buildID(containerName string, newImage string) string {
	return fmt.Sprintf("%s_%s", containerName, newImage)
}