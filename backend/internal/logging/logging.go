// Package logging configures the process-wide structured logger (log/slog).
// Terminal output is for developers and operators only — business audit
// records live in system_logs (internal/systemlog), the two are unrelated.
package logging

import (
	"log/slog"
	"os"
)

// L is the process-wide logger. It is assigned by Init before any request
// handling starts; packages read it at call time so they always see the
// configured handler.
var L = slog.Default()

// Init configures the structured logger from env vars and must be called
// first in main. LOG_LEVEL: debug|info|warn|error (default info, invalid
// values fall back to info). LOG_FORMAT: text|json (default json when
// APP_ENV=production — json suits container log collection). Both formats
// write to stdout, which container platforms surface.
func Init() {
	level := parseLevel(envOrDefault("LOG_LEVEL", "info"))
	format := envOrDefault("LOG_FORMAT", defaultFormat())
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	L = slog.New(h)
	slog.SetDefault(L)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func defaultFormat() string {
	if appEnv() == "production" {
		return "json"
	}
	return "text"
}

func appEnv() string {
	if v := os.Getenv("APP_ENV"); v != "" {
		return v
	}
	return "production"
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
