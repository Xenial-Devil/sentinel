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
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment is a Slack message attachment
type Attachment struct {
	Color  string `json:"color"`
	Text   string `json:"text"`
	Footer string `json:"footer"`
	Ts     int64  `json:"ts"`
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

// Send sends a notification to Slack
func (s *SlackNotifier) Send(n Notification, message string) error {
	msg := SlackMessage{
		Attachments: []Attachment{
			{
				Color:  getSlackColor(n.Event),
				Text:   message,
				Footer: "Sentinel 🛡️",
				Ts:     time.Now().Unix(),
			},
		},
	}

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