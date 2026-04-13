package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sentinel/logger"
	"time"
)

// SlackNotifier sends notifications to Slack
type SlackNotifier struct {
	WebhookURL string
	HTTPClient *http.Client
}

// SlackMessage is the Slack webhook payload
type SlackMessage struct {
	Blocks []SlackBlock `json:"blocks,omitempty"`
	Text   string       `json:"text"` // fallback for notifications
}

// SlackBlock is a Slack block kit block
type SlackBlock struct {
	Type string       `json:"type"`
	Text *SlackText   `json:"text,omitempty"`
	Fields []SlackText `json:"fields,omitempty"`
}

// SlackText is a Slack text object
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewSlack creates a new SlackNotifier
func NewSlack(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a notification to Slack using block kit
func (s *SlackNotifier) Send(n Notification, message string) error {
	msg := buildSlackMessage(n, message)

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %v", err)
	}

	resp, err := s.HTTPClient.Post(
		s.WebhookURL,
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to send slack message: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close slack response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status: %d", resp.StatusCode)
	}

	logger.Log.Debug("Slack notification sent")
	return nil
}

// buildSlackMessage builds a rich Slack block kit message
func buildSlackMessage(n Notification, message string) SlackMessage {
	icon := eventIcon(n.Event)
	color := getSlackColor(n.Event)
	_ = color // used in attachment mode if needed

	blocks := []SlackBlock{
		// Header
		{
			Type: "header",
			Text: &SlackText{
				Type: "plain_text",
				Text: fmt.Sprintf("%s Sentinel Alert", icon),
			},
		},
		// Main message
		{
			Type: "section",
			Text: &SlackText{
				Type: "mrkdwn",
				Text: message,
			},
		},
	}

	// Add fields if container info present
	if n.ContainerName != "" {
		fields := []SlackText{
			{Type: "mrkdwn", Text: fmt.Sprintf("*Event*\n%s", n.Event)},
			{Type: "mrkdwn", Text: fmt.Sprintf("*Container*\n%s", n.ContainerName)},
		}

		if n.Image != "" {
			fields = append(fields, SlackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Image*\n%s", n.Image),
			})
		}

		if n.Error != "" {
			fields = append(fields, SlackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Error*\n%s", n.Error),
			})
		}

		blocks = append(blocks, SlackBlock{
			Type:   "section",
			Fields: fields,
		})
	}

	// Divider
	blocks = append(blocks, SlackBlock{Type: "divider"})

	// Context footer
	blocks = append(blocks, SlackBlock{
		Type: "context",
		Text: &SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("Sentinel 🛡️  |  %s", formatTime(n.Timestamp)),
		},
	})

	return SlackMessage{
		Text:   fmt.Sprintf("%s %s - %s", icon, n.Event, n.ContainerName),
		Blocks: blocks,
	}
}

// getSlackColor returns color based on event type
func getSlackColor(event EventType) string {
	switch event {
	case EventUpdateSuccess:
		return "good"
	case EventUpdateFailed:
		return "danger"
	case EventRollback:
		return "warning"
	case EventHealthFailed:
		return "danger"
	default:
		return "#439FE0"
	}
}