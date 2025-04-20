package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/spf13/viper"

	"git.sgw.equipment/restricted/gcs_antal/internal/auth"
	"git.sgw.equipment/restricted/gcs_antal/internal/server"
)

func init() {
	// Set up configuration
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	// Read configuration
	if err := viper.ReadInConfig(); err != nil {
		slog.Error("Failed to read config file", "error", err)
		os.Exit(1)
	}

	// Configure logging
	logLevel := slog.LevelInfo
	if levelStr := viper.GetString("logging.level"); levelStr != "" {
		switch levelStr {
		case "debug":
			logLevel = slog.LevelDebug
		case "info":
			logLevel = slog.LevelInfo
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		}
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))

	// Initialize Sentry if configured
	if dsn := viper.GetString("sentry.dsn"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Environment:      viper.GetString("sentry.environment"),
			TracesSampleRate: viper.GetFloat64("sentry.sample_rate"),
		})
		if err != nil {
			slog.Error("Failed to initialize Sentry", "error", err)
		} else {
			slog.Info("Sentry initialized successfully")
		}
	}
}

func main() {
	logger := slog.With("component", "main")
	logger.Info("Starting NATS-GitLab Authentication Service")

	// Create GitLab client
	gitlabClient := auth.NewGitLabClient()

	// Create NATS client
	natsClient, err := auth.NewNATSClient(
		viper.GetString("nats.url"),
		viper.GetString("nats.user"),
		viper.GetString("nats.pass"),
		viper.GetString("auth.issuer_seed"),
		viper.GetString("auth.xkey_seed"),
		gitlabClient,
	)
	if err != nil {
		logger.Error("Failed to create NATS client", "error", err)
		os.Exit(1)
	}

	// Start NATS client
	if err := natsClient.Start(); err != nil {
		logger.Error("Failed to start NATS client", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	srv := server.NewServer(
		viper.GetString("server.host"),
		viper.GetInt("server.port"),
		time.Duration(viper.GetInt("server.timeout"))*time.Second,
	)

	// Start HTTP server in a goroutine
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start HTTP server", "error", err)
			os.Exit(1)
		}
	}()

	// Set up signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-quit
	logger.Info("Shutting down server...")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop HTTP server
	if err := srv.Stop(ctx); err != nil {
		logger.Error("Server shutdown failed", "error", err)
	}

	// Stop NATS client
	natsClient.Stop()

	// Flush sentry events
	sentry.Flush(2 * time.Second)

	logger.Info("Server exited properly")
}
