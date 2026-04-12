package logger

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// ANSI codes
const (
	colorReset    = "\033[0m"
	colorBold     = "\033[1m"
	colorDim      = "\033[2m"
	colorRed      = "\033[38;5;203m"
	colorGreen    = "\033[38;5;120m"
	colorYellow   = "\033[38;5;221m"
	colorBlue     = "\033[38;5;75m"
	colorPurple   = "\033[38;5;177m"
	colorCyan     = "\033[38;5;87m"
	colorOrange   = "\033[38;5;215m"
	colorGray     = "\033[38;5;245m"
	colorDarkGray = "\033[38;5;238m"
	colorWhite    = "\033[38;5;255m"
	bgRed         = "\033[48;5;52m"
	bgYellow      = "\033[48;5;58m"
)

// SentinelFormatter is a custom pretty log formatter
type SentinelFormatter struct{}

// Format renders a single log entry
func (f *SentinelFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b bytes.Buffer

	// ── Timestamp ────────────────────────────────────────────────────────────
	ts := entry.Time.Format("2006-01-02 15:04:05.000")
	b.WriteString(colorDarkGray + ts + colorReset)
	b.WriteString("  ")

	// ── Level badge ──────────────────────────────────────────────────────────
	b.WriteString(levelBadge(entry.Level))
	b.WriteString("  ")

	// ── Message ──────────────────────────────────────────────────────────────
	b.WriteString(msgColor(entry.Level))
	b.WriteString(entry.Message)
	b.WriteString(colorReset)

	// ── Fields ───────────────────────────────────────────────────────────────
	if len(entry.Data) > 0 {
		b.WriteString(colorDarkGray + "  │" + colorReset)

		// Sort fields: priority keys first
		keys := sortedKeys(entry.Data)

		for _, k := range keys {
			v := entry.Data[k]
			b.WriteString("  ")
			b.WriteString(fieldColor(k))
			b.WriteString(k)
			b.WriteString(colorDarkGray + "=" + colorReset)
			b.WriteString(valueColor(k, entry.Level))
			b.WriteString(fmt.Sprintf("%v", v))
			b.WriteString(colorReset)
		}
	}

	b.WriteString("\n")
	return b.Bytes(), nil
}

// levelBadge returns styled level indicator
func levelBadge(level logrus.Level) string {
	switch level {
	case logrus.TraceLevel:
		return colorDim + colorGray + "[ TRC ]" + colorReset
	case logrus.DebugLevel:
		return colorCyan + "[ DBG ]" + colorReset
	case logrus.InfoLevel:
		return colorBold + colorGreen + "[ INF ]" + colorReset
	case logrus.WarnLevel:
		return colorBold + colorYellow + "[ WRN ]" + colorReset
	case logrus.ErrorLevel:
		return colorBold + colorRed + "[ ERR ]" + colorReset
	case logrus.FatalLevel:
		return colorBold + bgRed + colorWhite + "[ FTL ]" + colorReset
	case logrus.PanicLevel:
		return colorBold + bgRed + colorWhite + "[ PNC ]" + colorReset
	default:
		return "[ INF ]"
	}
}

// msgColor returns message color based on level
func msgColor(level logrus.Level) string {
	switch level {
	case logrus.TraceLevel:
		return colorGray
	case logrus.DebugLevel:
		return colorGray
	case logrus.InfoLevel:
		return colorWhite
	case logrus.WarnLevel:
		return colorYellow
	case logrus.ErrorLevel:
		return colorRed
	case logrus.FatalLevel:
		return colorBold + colorRed
	default:
		return colorWhite
	}
}

// fieldColor returns key color based on field name
func fieldColor(key string) string {
	switch key {
	case "container":
		return colorCyan + colorBold
	case "image", "old_image", "new_image":
		return colorBlue
	case "component":
		return colorPurple
	case "error":
		return colorRed + colorBold
	case "duration", "took":
		return colorOrange
	case "cycle":
		return colorPurple
	case "updated", "skipped", "failed", "total":
		return colorGreen
	case "tag", "current", "latest":
		return colorCyan
	case "caller":
		return colorDarkGray
	default:
		return colorGray
	}
}

// valueColor returns value color based on field name and level
func valueColor(key string, level logrus.Level) string {
	switch key {
	case "error":
		return colorRed
	case "container":
		return colorCyan
	case "image", "old_image", "new_image":
		return colorBlue
	case "component":
		return colorPurple
	case "duration", "took":
		return colorOrange
	case "current", "latest", "tag":
		return colorGreen
	default:
		if level >= logrus.WarnLevel {
			return colorYellow
		}
		return colorGray
	}
}

// priorityKeys defines field display order
var priorityKeys = []string{
	"component",
	"cycle",
	"container",
	"image",
	"old_image",
	"new_image",
	"current",
	"latest",
	"tag",
	"id",
	"status",
	"duration",
	"total",
	"updated",
	"skipped",
	"failed",
	"error",
	"caller",
}

// sortedKeys returns fields in priority order then alphabetical
func sortedKeys(data logrus.Fields) []string {
	seen := make(map[string]bool)
	var result []string

	// Priority keys first
	for _, k := range priorityKeys {
		if _, ok := data[k]; ok {
			result = append(result, k)
			seen[k] = true
		}
	}

	// Remaining keys alphabetically
	var rest []string
	for k := range data {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	result = append(result, rest...)

	return result
}

// separator returns a visual divider line
func separator() string {
	return colorDarkGray + strings.Repeat("─", 80) + colorReset
}