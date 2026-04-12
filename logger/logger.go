package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Log is the global logger instance
var Log = logrus.New()

// Init sets up the logger
func Init(level string, format string) {
	// Set log level
	switch level {
	case "debug":
		Log.SetLevel(logrus.DebugLevel)
	case "trace":
		Log.SetLevel(logrus.TraceLevel)
	case "warn":
		Log.SetLevel(logrus.WarnLevel)
	case "error":
		Log.SetLevel(logrus.ErrorLevel)
	default:
		Log.SetLevel(logrus.InfoLevel)
	}

	// Set log format
	switch format {
	case "json":
		Log.SetFormatter(&logrus.JSONFormatter{})
	case "logfmt":
		Log.SetFormatter(&logrus.TextFormatter{
			DisableColors: true,
		})
	default:
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true,
		})
	}

	// Output to stdout
	Log.SetOutput(os.Stdout)
}