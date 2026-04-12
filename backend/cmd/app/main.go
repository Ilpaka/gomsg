package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"goflow/backend/internal/app"
	"goflow/backend/internal/config"
	"goflow/backend/internal/pkg/logger"
	"goflow/backend/internal/repository/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New()
	log.Info("application starting",
		"http_port", cfg.App.Port,
		"redis_addr", cfg.Redis.Addr,
		"jwt_access_ttl_seconds", cfg.JWT.AccessTTLSeconds,
		"jwt_refresh_ttl_seconds", cfg.JWT.RefreshTTLSeconds,
	)

	pool, err := postgres.NewPool(context.Background(), cfg.Postgres.DSN)
	if err != nil {
		log.Error("postgres pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	container, err := app.NewContainer(cfg, log, pool)
	if err != nil {
		log.Error("failed to create container", "err", err)
		os.Exit(1)
	}
	defer container.Close()

	application, err := app.New(container)
	if err != nil {
		log.Error("failed to create application", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Error("application stopped with error", "err", err)
		os.Exit(1)
	}
	log.Info("application stopped")
}
