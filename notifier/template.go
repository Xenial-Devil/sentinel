package notifier

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// TemplateData holds all data available in notification templates
type TemplateData struct {
	// Event info
	Event     string
	Timestamp string

	// Container info
	Container string
	Image     string
	OldImage  string
	NewImage  string
	Error     string

	// Emoji helpers
	Icon string
}

// DefaultTemplates holds the default message templates per event
var DefaultTemplates = map[EventType]string{
	EventUpdateFound: `🔍 *Update Available*
Container : {{.Container}}
Image     : {{.Image}}
Time      : {{.Timestamp}}`,

	EventUpdateSuccess: `✅ *Container Updated*
Container : {{.Container}}
Old Image : {{.OldImage}}
New Image : {{.NewImage}}
Time      : {{.Timestamp}}`,

	EventUpdateFailed: `❌ *Update Failed*
Container : {{.Container}}
Image     : {{.Image}}
Error     : {{.Error}}
Time      : {{.Timestamp}}`,

	EventRollback: `⚠️ *Container Rolled Back*
Container : {{.Container}}
From      : {{.NewImage}}
To        : {{.OldImage}}
Time      : {{.Timestamp}}`,

	EventHealthFailed: `🚨 *Health Check Failed*
Container : {{.Container}}
Image     : {{.Image}}
Time      : {{.Timestamp}}`,

	EventStartup: `🛡️ *Sentinel Started*
Time      : {{.Timestamp}}
Watching containers...`,
}

// TemplateEngine renders notification messages from templates
type TemplateEngine struct {
	templates map[EventType]string
}

// NewTemplateEngine creates a template engine with default templates
func NewTemplateEngine() *TemplateEngine {
	t := &TemplateEngine{
		templates: make(map[EventType]string),
	}

	// Copy defaults
	for k, v := range DefaultTemplates {
		t.templates[k] = v
	}

	return t
}

// SetTemplate overrides a template for a specific event
func (t *TemplateEngine) SetTemplate(event EventType, tmpl string) {
	t.templates[event] = tmpl
}

// Render renders a notification message using the template for the event
func (t *TemplateEngine) Render(n Notification) string {
	tmplStr, ok := t.templates[n.Event]
	if !ok {
		// Fallback to generic
		return fmt.Sprintf("📢 Sentinel: %s | Container: %s", n.Event, n.ContainerName)
	}

	data := TemplateData{
		Event:     string(n.Event),
		Timestamp: formatTime(n.Timestamp),
		Container: n.ContainerName,
		Image:     n.Image,
		OldImage:  n.OldImage,
		NewImage:  n.NewImage,
		Error:     n.Error,
		Icon:      eventIcon(n.Event),
	}

	tmpl, err := template.New("notification").Parse(tmplStr)
	if err != nil {
		return fmt.Sprintf("📢 Sentinel: %s | Container: %s (template error: %v)",
			n.Event, n.ContainerName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("📢 Sentinel: %s | Container: %s (render error: %v)",
			n.Event, n.ContainerName, err)
	}

	return strings.TrimSpace(buf.String())
}

// formatTime formats a time for display
func formatTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format("2006-01-02 15:04:05 UTC")
}

// eventIcon returns an emoji icon for an event
func eventIcon(event EventType) string {
	switch event {
	case EventUpdateFound:
		return "🔍"
	case EventUpdateSuccess:
		return "✅"
	case EventUpdateFailed:
		return "❌"
	case EventRollback:
		return "⚠️"
	case EventHealthFailed:
		return "🚨"
	case EventStartup:
		return "🛡️"
	default:
		return "📢"
	}
}