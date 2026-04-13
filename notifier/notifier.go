package notifier

import (
	"sentinel/config"
	"sentinel/logger"
	"time"
)

// EventType defines notification event types
type EventType string

const (
	EventUpdateFound   EventType = "update_found"
	EventUpdateSuccess EventType = "update_success"
	EventUpdateFailed  EventType = "update_failed"
	EventRollback      EventType = "rollback"
	EventHealthFailed  EventType = "health_failed"
	EventStartup       EventType = "startup"
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
	Template *TemplateEngine
}

// New creates a new Notifier with all configured channels
func New(cfg *config.Config) *Notifier {
	n := &Notifier{
		Config:   cfg,
		Template: NewTemplateEngine(),
	}

	// Override templates from env if configured
	loadTemplatesFromConfig(n.Template, cfg)

	if cfg.SlackWebhook != "" {
		n.Slack = NewSlack(cfg.SlackWebhook)
		logger.Log.WithField("channel", "slack").Info("📣  Slack notifications enabled")
	}

	if cfg.EmailTo != "" {
		n.Email = NewEmail(cfg)
		logger.Log.WithField("channel", "email").Info("📣  Email notifications enabled")
	}

	if cfg.TeamsWebhook != "" {
		n.Teams = NewTeams(cfg.TeamsWebhook)
		logger.Log.WithField("channel", "teams").Info("📣  Teams notifications enabled")
	}

	return n
}

// Send sends notification to all configured channels using templates
func (n *Notifier) Send(notification Notification) {
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	// Render message using template engine
	message := n.Template.Render(notification)

	logger.Log.WithFields(logger.Fields{
		"event":     notification.Event,
		"container": notification.ContainerName,
	}).Debugf("Sending notification via all channels")

	if n.Slack != nil {
		if err := n.Slack.Send(notification, message); err != nil {
			logger.Log.WithError(err).Error("Failed to send Slack notification")
		}
	}

	if n.Email != nil {
		if err := n.Email.Send(notification, message); err != nil {
			logger.Log.WithError(err).Error("Failed to send Email notification")
		}
	}

	if n.Teams != nil {
		if err := n.Teams.Send(notification, message); err != nil {
			logger.Log.WithError(err).Error("Failed to send Teams notification")
		}
	}
}

// loadTemplatesFromConfig loads custom templates from config env vars
func loadTemplatesFromConfig(t *TemplateEngine, cfg *config.Config) {
	// Custom templates can be set via NotifyTemplates map in config
	for event, tmpl := range cfg.NotifyTemplates {
		t.SetTemplate(EventType(event), tmpl)
		logger.Log.WithField("event", event).Debug("📝  Custom notification template loaded")
	}
}

// ── Convenience helpers ───────────────────────────────────────────────────────

func (n *Notifier) NotifyUpdateFound(containerName string, image string) {
	n.Send(Notification{
		Event:         EventUpdateFound,
		ContainerName: containerName,
		Image:         image,
	})
}

func (n *Notifier) NotifySuccess(containerName string, oldImage string, newImage string) {
	n.Send(Notification{
		Event:         EventUpdateSuccess,
		ContainerName: containerName,
		OldImage:      oldImage,
		NewImage:      newImage,
	})
}

func (n *Notifier) NotifyFailed(containerName string, image string, err error) {
	n.Send(Notification{
		Event:         EventUpdateFailed,
		ContainerName: containerName,
		Image:         image,
		Error:         err.Error(),
	})
}

func (n *Notifier) NotifyRollback(containerName string, oldImage string, newImage string) {
	n.Send(Notification{
		Event:         EventRollback,
		ContainerName: containerName,
		OldImage:      oldImage,
		NewImage:      newImage,
	})
}

func (n *Notifier) NotifyHealthFailed(containerName string, image string) {
	n.Send(Notification{
		Event:         EventHealthFailed,
		ContainerName: containerName,
		Image:         image,
	})
}

func (n *Notifier) NotifyStartup() {
	n.Send(Notification{
		Event: EventStartup,
	})
}