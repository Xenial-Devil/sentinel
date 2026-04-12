package notifier

import (
	"fmt"
	"net/smtp"
	"sentinel/config"
	"sentinel/logger"
)

// EmailNotifier sends notifications via email
type EmailNotifier struct {
	To       string
	From     string
	Host     string
	Port     string
	Username string
	Password string
}

// NewEmail creates a new EmailNotifier
func NewEmail(cfg *config.Config) *EmailNotifier {
	return &EmailNotifier{
		To:       cfg.EmailTo,
		From:     cfg.EmailTo,
		Host:     "smtp.gmail.com",
		Port:     "587",
		Username: "",
		Password: "",
	}
}

// Send sends an email notification
func (e *EmailNotifier) Send(n Notification, message string) error {
	// Build email
	subject := getEmailSubject(n.Event, n.ContainerName)
	body := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, message)

	// Setup auth
	auth := smtp.PlainAuth(
		"",
		e.Username,
		e.Password,
		e.Host,
	)

	// Send email
	err := smtp.SendMail(
		e.Host+":"+e.Port,
		auth,
		e.From,
		[]string{e.To},
		[]byte(body),
	)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	logger.Log.Debug("Email notification sent")
	return nil
}

// getEmailSubject returns email subject based on event
func getEmailSubject(event EventType, containerName string) string {
	switch event {
	case EventUpdateSuccess:
		return fmt.Sprintf("✅ Sentinel: %s updated successfully", containerName)
	case EventUpdateFailed:
		return fmt.Sprintf("❌ Sentinel: %s update failed", containerName)
	case EventRollback:
		return fmt.Sprintf("⚠️ Sentinel: %s rolled back", containerName)
	case EventHealthFailed:
		return fmt.Sprintf("🚨 Sentinel: %s health check failed", containerName)
	default:
		return fmt.Sprintf("🛡️ Sentinel: %s", event)
	}
}