package notifier

import (
	"fmt"
	"net/smtp"
	"sentinel/config"
	"sentinel/logger"
	"strings"
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
		From:     cfg.EmailFrom,
		Host:     cfg.EmailHost,
		Port:     cfg.EmailPort,
		Username: cfg.EmailUsername,
		Password: cfg.EmailPassword,
	}
}

// Send sends an HTML email notification
func (e *EmailNotifier) Send(n Notification, message string) error {
	subject := getEmailSubject(n.Event, n.ContainerName)
	body := buildEmailBody(n, message)

	// Build MIME message
	mime := strings.Join([]string{
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		fmt.Sprintf("From: %s", e.From),
		fmt.Sprintf("To: %s", e.To),
		fmt.Sprintf("Subject: %s", subject),
		"",
		body,
	}, "\r\n")

	auth := smtp.PlainAuth("", e.Username, e.Password, e.Host)

	if err := smtp.SendMail(
		e.Host+":"+e.Port,
		auth,
		e.From,
		[]string{e.To},
		[]byte(mime),
	); err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	logger.Log.Debug("Email notification sent")
	return nil
}

// buildEmailBody builds a rich HTML email body
func buildEmailBody(n Notification, message string) string {
	icon := eventIcon(n.Event)
	color := getEmailColor(n.Event)

	rows := ""
	if n.ContainerName != "" {
		rows += emailRow("Container", n.ContainerName)
	}
	if n.Image != "" {
		rows += emailRow("Image", n.Image)
	}
	if n.OldImage != "" {
		rows += emailRow("Old Image", n.OldImage)
	}
	if n.NewImage != "" {
		rows += emailRow("New Image", n.NewImage)
	}
	if n.Error != "" {
		rows += emailRow("Error", fmt.Sprintf(`<span style="color:#cc0000">%s</span>`, n.Error))
	}
	rows += emailRow("Time", formatTime(n.Timestamp))

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:Arial,sans-serif;background:#f4f4f4;padding:20px">
  <div style="max-width:600px;margin:0 auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.1)">
    <div style="background:%s;padding:20px;text-align:center">
      <h1 style="color:#fff;margin:0;font-size:24px">%s Sentinel Alert</h1>
    </div>
    <div style="padding:24px">
      <p style="font-size:16px;color:#333">%s</p>
      <table style="width:100%%;border-collapse:collapse;margin-top:16px">
        %s
      </table>
    </div>
    <div style="background:#f8f8f8;padding:12px;text-align:center;font-size:12px;color:#999">
      Sentinel 🛡️ - Docker Auto-Updater
    </div>
  </div>
</body>
</html>`, color, icon, message, rows)
}

// emailRow builds a table row for the email
func emailRow(label, value string) string {
	return fmt.Sprintf(`
    <tr>
      <td style="padding:8px;border-bottom:1px solid #eee;font-weight:bold;color:#555;width:130px">%s</td>
      <td style="padding:8px;border-bottom:1px solid #eee;color:#333">%s</td>
    </tr>`, label, value)
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
	case EventStartup:
		return "🛡️ Sentinel: started"
	default:
		return fmt.Sprintf("🛡️ Sentinel: %s", event)
	}
}

// getEmailColor returns header color based on event
func getEmailColor(event EventType) string {
	switch event {
	case EventUpdateSuccess:
		return "#28a745"
	case EventUpdateFailed:
		return "#dc3545"
	case EventRollback:
		return "#fd7e14"
	case EventHealthFailed:
		return "#dc3545"
	default:
		return "#0078D7"
	}
}