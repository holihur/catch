package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/catch/catch/backend/internal/api"
	"github.com/catch/catch/backend/internal/config"
	"github.com/catch/catch/backend/internal/logging"
	"github.com/catch/catch/backend/internal/seed"
	"github.com/catch/catch/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	st, err := store.Connect(ctx, cfg.MongoURI, cfg.MongoDB, cfg.Retention)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() {
		closeCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = st.Close(closeCtx)
	}()

	logger := logging.New(st, logging.Options{
		Level: cfg.LogLevel, Service: "catch-api", Persist: true, QueueSize: 4096,
	})
	defer logger.Close()
	log := logger.Slog

	if err := seed.Run(ctx, st, cfg, log); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	a, err := api.New(cfg, st, logger)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           a.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("catch api listening",
			"port", cfg.Port, "env", cfg.Env, "version", api.Version,
			"publicUrl", cfg.PublicURL, "appUrl", cfg.AppURL,
			"devLogin", cfg.AllowDev, "staticDir", cfg.StaticDir,
			"db", cfg.MongoDB, "retentionDays", int(cfg.Retention.Hours()/24))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case s := <-sig:
		log.Info("shutting down", "signal", s.String())
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		return err
	}
	log.Info("server stopped", "droppedLogs", logger.Dropped(), "failedLogs", logger.Failed())
	return nil
}
