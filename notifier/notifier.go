package notifier

import (
	"fmt"
	"sentinel/config"
	"sentinel/logger"
	"time"
)

// Event types for notifications
type EventType string

const (
	EventUpdateFound    EventType = "update_found"
	EventUpdateSuccess  EventType = "update_success"
	EventUpdateFailed   EventType = "update_failed"
	EventRollback       EventType = "rollback"
	EventHealthFailed   EventType = "health_failed"
	EventStartup        EventType = "startup"
)

// Notification holds all notification data
type Notification struct {
	Event         EventType
	ContainerName string
	Image         string
	OldImage      string
	NewImage      string
	Error         string
	Timestamp     time.Time
}

// Notifier is the base notification manager
type Notifier struct {
	Config   *config.Config
	Slack    *SlackNotifier
	Email    *EmailNotifier
	Teams    *TeamsNotifier
}

// New creates a new Notifier
func New(cfg *config.Config) *Notifier {
	n := &Notifier{
		Config: cfg,
	}

	// Setup slack if configured
	if cfg.SlackWebhook != "" {
		n.Slack = NewSlack(cfg.SlackWebhook)
		logger.Log.Info("Slack notifications enabled")
	}

	// Setup email if configured
	if cfg.EmailTo != "" {
		n.Email = NewEmail(cfg)
		logger.Log.Info("Email notifications enabled")
	}

	return n
}

// Send sends notification to all configured channels
func (n *Notifier) Send(notification Notification) {
	// Set timestamp if not set
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	// Format message
	message := n.formatMessage(notification)

	logger.Log.Debugf("Sending notification: %s", message)

	// Send to slack
	if n.Slack != nil {
		err := n.Slack.Send(notification, message)
		if err != nil {
			logger.Log.Errorf("Failed to send Slack notification: %v", err)
		}
	}

	// Send to email
	if n.Email != nil {
		err := n.Email.Send(notification, message)
		if err != nil {
			logger.Log.Errorf("Failed to send Email notification: %v", err)
		}
	}

	// Send to teams
	if n.Teams != nil {
		err := n.Teams.Send(notification, message)
		if err != nil {
			logger.Log.Errorf("Failed to send Teams notification: %v", err)
		}
	}
}

// formatMessage formats notification into readable message
func (n *Notifier) formatMessage(notification Notification) string {
	switch notification.Event {

	case EventUpdateFound:
		return fmt.Sprintf(
			"🔍 Update found for %s\nImage: %s",
			notification.ContainerName,
			notification.Image,
		)

	case EventUpdateSuccess:
		return fmt.Sprintf(
			"✅ Successfully updated %s\nImage: %s",
			notification.ContainerName,
			notification.NewImage,
		)

	case EventUpdateFailed:
		return fmt.Sprintf(
			"❌ Failed to update %s\nImage: %s\nError: %s",
			notification.ContainerName,
			notification.Image,
			notification.Error,
		)

	case EventRollback:
		return fmt.Sprintf(
			"⚠️ Rolled back %s\nFrom: %s\nTo: %s",
			notification.ContainerName,
			notification.NewImage,
			notification.OldImage,
		)

	case EventHealthFailed:
		return fmt.Sprintf(
			"🚨 Health check failed for %s\nImage: %s",
			notification.ContainerName,
			notification.Image,
		)

	case EventStartup:
		return "🛡️ Sentinel started\nWatching containers..."

	default:
		return fmt.Sprintf(
			"📢 Sentinel event: %s\nContainer: %s",
			notification.Event,
			notification.ContainerName,
		)
	}
}

// NotifyUpdate sends update found notification
func (n *Notifier) NotifyUpdate(containerName string, image string) {
	n.Send(Notification{
		Event:         EventUpdateFound,
		ContainerName: containerName,
		Image:         image,
	})
}

// NotifySuccess sends update success notification
func (n *Notifier) NotifySuccess(containerName string, newImage string) {
	n.Send(Notification{
		Event:         EventUpdateSuccess,
		ContainerName: containerName,
		NewImage:      newImage,
	})
}

// NotifyFailed sends update failed notification
func (n *Notifier) NotifyFailed(containerName string, image string, err error) {
	n.Send(Notification{
		Event:         EventUpdateFailed,
		ContainerName: containerName,
		Image:         image,
		Error:         err.Error(),
	})
}

// NotifyRollback sends rollback notification
func (n *Notifier) NotifyRollback(containerName string, oldImage string, newImage string) {
	n.Send(Notification{
		Event:         EventRollback,
		ContainerName: containerName,
		OldImage:      oldImage,
		NewImage:      newImage,
	})
}

// NotifyStartup sends startup notification
func (n *Notifier) NotifyStartup() {
	n.Send(Notification{
		Event: EventStartup,
	})
}