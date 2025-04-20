package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sgw.equipment/restricted/gcs_antal/internal/server"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/viper"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	if err := loadConfig(*configPath); err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize Sentry if DSN is provided
	if dsn := viper.GetString("sentry.dsn"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Environment:      viper.GetString("sentry.environment"),
			TracesSampleRate: viper.GetFloat64("sentry.traces_sample_rate"),
		})
		if err != nil {
			logger.Warn("Sentry initialization failed", "error", err)
		} else {
			logger.Info("Sentry initialized successfully")
			defer sentry.Flush(2 * time.Second)
		}
	} else {
		logger.Info("Sentry DSN not provided, error tracking disabled")
	}

	// Create and start the server
	srv, err := server.NewServer()
	if err != nil {
		logger.Error("Failed to initialize server", "error", err)
		sentry.CaptureException(err)
		os.Exit(1)
	}

	// Start server in a goroutine
	go func() {
		addr := fmt.Sprintf("%s:%d", viper.GetString("server.host"), viper.GetInt("server.port"))
		logger.Info("Starting server", "address", addr)
		if err := srv.Start(addr); err != nil {
			logger.Error("Server failed", "error", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// Create a deadline to wait for current operations to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		sentry.CaptureException(err)
	}

	logger.Info("Server exited properly")
}

func loadConfig(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set default values
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.timeout", 10) // seconds
	viper.SetDefault("gitlab.url", "https://git.sgw.equipment")
	viper.SetDefault("gitlab.timeout", 5) // seconds
	viper.SetDefault("logging.level", "info")

	// Read environment variables prefixed with GCS_ANTAL_
	viper.SetEnvPrefix("GCS_ANTAL")
	viper.AutomaticEnv()

	// Read the configuration file
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	return nil
}
