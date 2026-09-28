package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/jobs"
	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/research"
	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := jobs.Open(ctx, required("REDIS_URL"))
	if err != nil {
		logger.Error("open job store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.EnsureConsumerGroup(ctx); err != nil {
		logger.Error("prepare consumer group", "error", err)
		os.Exit(1)
	}

	agent, err := research.NewCodex(os.Getenv("CODEX_MODEL"))
	if err != nil {
		logger.Error("create research agent", "error", err)
		os.Exit(1)
	}

	pool, err := worker.NewPool(
		store,
		agent,
		integer("WORKER_CONCURRENCY", 1),
		value("HOSTNAME", "worker"),
		duration("RESEARCH_TIMEOUT", 5*time.Minute),
		logger,
	)
	if err != nil {
		logger.Error("create worker pool", "error", err)
		os.Exit(1)
	}

	if err := pool.Run(ctx); err != nil {
		logger.Error("run worker pool", "error", err)
		os.Exit(1)
	}
}

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		slog.Error("required environment variable is missing", "name", name)
		os.Exit(1)
	}
	return value
}

func value(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func integer(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		slog.Error("invalid integer environment variable", "name", name, "error", err)
		os.Exit(1)
	}
	return parsed
}

func duration(name string, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		slog.Error("invalid duration environment variable", "name", name, "error", err)
		os.Exit(1)
	}
	return parsed
}
