package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "github.com/simonbalfe/k3s-hetzner-adk-research/internal/api"
	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/jobs"
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

	api, err := apihttp.New(store, required("API_TOKEN"), logger)
	if err != nil {
		logger.Error("create API", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              value("HTTP_ADDR", ":8080"),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve API", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shut down API", "error", err)
			os.Exit(1)
		}
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
