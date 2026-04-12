package webhook

import "time"

// EventType defines webhook event types
type EventType string

const (
	EventUpdateDetected  EventType = "update.detected"
	EventPullStarted     EventType = "pull.started"
	EventPullFailed      EventType = "pull.failed"
	EventRecreateStarted EventType = "recreate.started"
	EventRecreateSuccess EventType = "recreate.success"
	EventHealthFailed    EventType = "health.failed"
	EventRollbackDone    EventType = "rollback.done"
	EventApprovalPending EventType = "approval.pending"
)

// Payload is the webhook payload sent to external systems
type Payload struct {
	Event         EventType  `json:"event"`
	Timestamp     time.Time  `json:"timestamp"`
	ContainerName string     `json:"container_name"`
	Image         string     `json:"image"`
	OldImage      string     `json:"old_image,omitempty"`
	NewImage      string     `json:"new_image,omitempty"`
	Error         string     `json:"error,omitempty"`
	Meta          PayloadMeta `json:"meta"`
}

// PayloadMeta holds extra metadata
type PayloadMeta struct {
	Host    string `json:"host"`
	Version string `json:"version"`
}