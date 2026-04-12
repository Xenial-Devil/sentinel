package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sentinel/logger"
	"time"
)

// TeamsNotifier sends notifications to Microsoft Teams
type TeamsNotifier struct {
	WebhookURL string
	HTTPClient *http.Client
}

// TeamsMessage is the Teams webhook payload
type TeamsMessage struct {
	Type       string    `json:"@type"`
	Context    string    `json:"@context"`
	ThemeColor string    `json:"themeColor"`
	Summary    string    `json:"summary"`
	Sections   []Section `json:"sections"`
}

// Section is a Teams message section
type Section struct {
	ActivityTitle string `json:"activityTitle"`
	ActivityText  string `json:"activityText"`
}

// NewTeams creates a new TeamsNotifier
func NewTeams(webhookURL string) *TeamsNotifier {
	return &TeamsNotifier{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a notification to Teams
func (t *TeamsNotifier) Send(n Notification, message string) error {
	// Build teams message
	msg := TeamsMessage{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: getTeamsColor(n.Event),
		Summary:    "Sentinel Notification",
		Sections: []Section{
			{
				ActivityTitle: fmt.Sprintf("Sentinel 🛡️ - %s", n.Event),
				ActivityText:  message,
			},
		},
	}

	// Marshal to JSON
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal teams message: %v", err)
	}

	// Send request
	resp, err := t.HTTPClient.Post(
		t.WebhookURL,
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to send teams message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("teams returned status: %d", resp.StatusCode)
	}

	logger.Log.Debug("Teams notification sent")
	return nil
}

// getTeamsColor returns color based on event type
func getTeamsColor(event EventType) string {
	switch event {
	case EventUpdateSuccess:
		return "00FF00" // green
	case EventUpdateFailed:
		return "FF0000" // red
	case EventRollback:
		return "FFA500" // orange
	case EventHealthFailed:
		return "FF0000" // red
	default:
		return "0078D7" // blue
	}
}