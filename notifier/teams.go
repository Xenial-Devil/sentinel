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

// TeamsMessage is the Teams adaptive card payload
type TeamsMessage struct {
	Type       string         `json:"@type"`
	Context    string         `json:"@context"`
	ThemeColor string         `json:"themeColor"`
	Summary    string         `json:"summary"`
	Sections   []TeamsSection `json:"sections"`
}

// TeamsSection is a Teams message section
type TeamsSection struct {
	ActivityTitle    string      `json:"activityTitle"`
	ActivitySubtitle string      `json:"activitySubtitle"`
	ActivityText     string      `json:"activityText,omitempty"`
	Facts            []TeamsFact `json:"facts,omitempty"`
	Markdown         bool        `json:"markdown"`
}

// TeamsFact is a key-value fact in Teams
type TeamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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
	msg := buildTeamsMessage(n, message)

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal teams message: %v", err)
	}

	resp, err := t.HTTPClient.Post(
		t.WebhookURL,
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to send teams message: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close teams response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("teams returned status: %d", resp.StatusCode)
	}

	logger.Log.Debug("Teams notification sent")
	return nil
}

// buildTeamsMessage builds a rich Teams message card
func buildTeamsMessage(n Notification, message string) TeamsMessage {
	icon := eventIcon(n.Event)

	// Build facts
	facts := []TeamsFact{
		{Name: "Event", Value: string(n.Event)},
		{Name: "Time", Value: formatTime(n.Timestamp)},
	}

	if n.ContainerName != "" {
		facts = append(facts, TeamsFact{Name: "Container", Value: n.ContainerName})
	}
	if n.Image != "" {
		facts = append(facts, TeamsFact{Name: "Image", Value: n.Image})
	}
	if n.OldImage != "" {
		facts = append(facts, TeamsFact{Name: "Old Image", Value: n.OldImage})
	}
	if n.NewImage != "" {
		facts = append(facts, TeamsFact{Name: "New Image", Value: n.NewImage})
	}
	if n.Error != "" {
		facts = append(facts, TeamsFact{Name: "Error", Value: n.Error})
	}

	return TeamsMessage{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: getTeamsColor(n.Event),
		Summary:    fmt.Sprintf("Sentinel: %s - %s", n.Event, n.ContainerName),
		Sections: []TeamsSection{
			{
				ActivityTitle:    fmt.Sprintf("%s Sentinel Alert", icon),
				ActivitySubtitle: message,
				Facts:            facts,
				Markdown:         true,
			},
		},
	}
}

// getTeamsColor returns color based on event type
func getTeamsColor(event EventType) string {
	switch event {
	case EventUpdateSuccess:
		return "00FF00"
	case EventUpdateFailed:
		return "FF0000"
	case EventRollback:
		return "FFA500"
	case EventHealthFailed:
		return "FF0000"
	default:
		return "0078D7"
	}
}