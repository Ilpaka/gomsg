package logger

import (
	"log/slog"
	"os"
	"strings"

	"goflow/backend/internal/config"
)

// New returns a slog logger. When cfg is non-nil, observability fields and log format apply.
func New(cfg *config.Config) *slog.Logger {
	svc := "goflow-backend"
	env := "development"
	format := "text"
	if cfg != nil {
		if strings.TrimSpace(cfg.Observability.ServiceName) != "" {
			svc = strings.TrimSpace(cfg.Observability.ServiceName)
		}
		if strings.TrimSpace(cfg.Observability.Env) != "" {
			env = strings.TrimSpace(cfg.Observability.Env)
		}
		if strings.TrimSpace(cfg.Observability.LogFormat) != "" {
			format = strings.TrimSpace(cfg.Observability.LogFormat)
		}
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h).With(
		slog.String("service", svc),
		slog.String("env", env),
	)
}
