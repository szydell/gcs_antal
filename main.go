package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"git.sgw.equipment/restricted/gcs_antal/internal/auth"
	"git.sgw.equipment/restricted/gcs_antal/internal/server"
)

// Version information - can be set during build using:
// go build -ldflags "-X main.version=1.0.0" -o antal
var version = "dev"

func init() {
	// Define command line flags
	pflag.String("config", "", "Path to config file")
	pflag.Bool("version", false, "Display version information")
	pflag.Parse()

	// Check if a version flag is passed
	if versionFlag, _ := pflag.CommandLine.GetBool("version"); versionFlag {
		fmt.Printf("GCS Antal version: %s\n", version)
		os.Exit(0)
	}

	// Bind command line flags to viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		slog.Error("Failed to bind command line flags", "error", err)
		os.Exit(1)
	}

	// Set up configuration defaults
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	// Use custom a config file if specified
	if configFile := viper.GetString("config"); configFile != "" {
		viper.SetConfigFile(configFile)
	}

	// Read configuration
	if err := viper.ReadInConfig(); err != nil {
		// Handle config file error
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			slog.Error("Failed to read config file", "error", err)
			if viper.GetString("sentry.dsn") != "" {
				sentry.CaptureException(err)
				sentry.Flush(time.Second * 2)
			}
			os.Exit(1)
		}
	} else {
		slog.Info("Config loaded successfully", "file", viper.ConfigFileUsed())
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
			EnableTracing:    viper.GetBool("sentry.enable_tracing"),
			Debug:            viper.GetBool("sentry.debug"),
			AttachStacktrace: true,
		})
		if err != nil {
			slog.Error("Failed to initialize Sentry", "error", err)
		} else {
			slog.Info("Sentry initialized successfully",
				"environment", viper.GetString("sentry.environment"),
				"tracing_enabled", viper.GetBool("sentry.enable_tracing"))

			// Optional: test event showing configuration
			if viper.GetBool("sentry.debug") {
				sentry.AddBreadcrumb(&sentry.Breadcrumb{
					Category: "config",
					Message:  "Sentry configuration loaded",
					Level:    sentry.LevelInfo,
					Data: map[string]interface{}{
						"environment":     viper.GetString("sentry.environment"),
						"tracing_enabled": viper.GetBool("sentry.enable_tracing"),
						"sample_rate":     viper.GetFloat64("sentry.sample_rate"),
					},
				})
			}
		}
	} else {
		slog.Warn("Sentry DSN not provided - error tracking disabled")
	}
}

func main() {
	logger := slog.With("component", "main")
	logger.Info("Starting GCS Antal, a NATS-GitLab Authentication Service", "version", version)

	// Test event for Sentry
	if viper.GetString("sentry.dsn") != "" {
		sentry.CaptureMessage("GCS Antal started")
		sentry.Flush(time.Second * 5)
	}

	// Create a GitLab client
	gitlabClient := auth.NewGitLabClient()

	// Create a NATS client
	natsClient, err := auth.NewNATSClient(
		viper.GetString("nats.url"),
		viper.GetString("nats.user"),
		viper.GetString("nats.pass"),
		viper.GetString("nats.issuer_seed"),
		viper.GetString("nats.xkey_seed"),
		gitlabClient,
	)
	if err != nil {
		logger.Error("Failed to create NATS client", "error", err)
		os.Exit(1)
	}

	// Start the NATS client
	if err := natsClient.Start(); err != nil {
		logger.Error("Failed to start NATS client", "error", err)
		os.Exit(1)
	}

	// Create an HTTP server
	srv := server.NewServer(
		viper.GetString("server.host"),
		viper.GetInt("server.port"),
		time.Duration(viper.GetInt("server.timeout"))*time.Second,
	)

	// Start an HTTP server in a goroutine
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Failed to start HTTP server", "error", err)
			os.Exit(1)
		}
	}()

	// Set up signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for the interrupt signal
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
