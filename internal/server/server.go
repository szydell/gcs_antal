package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"git.sgw.equipment/restricted/gcs_antal/internal/auth"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/spf13/viper"
)

// Server represents the HTTP server
type Server struct {
	server *http.Server
}

// NewServer creates a new HTTP server with configured routes
func NewServer() (*Server, error) {
	// Create router using standard library
	mux := http.NewServeMux()

	// Create GitLab auth handler
	authHandler, err := auth.NewHandler()
	if err != nil {
		return nil, err
	}

	// Register routes
	mux.HandleFunc("POST /auth", authHandler.HandleAuth)

	// Add health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Configure the HTTP server
	timeoutDuration := time.Duration(viper.GetInt("server.timeout")) * time.Second
	var handler http.Handler = mux

	// Add Sentry middleware if configured
	if viper.GetString("sentry.dsn") != "" {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		handler = sentryHandler.Handle(handler)
	}

	// Add logging middleware
	handler = logMiddleware(handler)

	// Configure the HTTP server
	srv := &http.Server{
		Handler:      handler,
		ReadTimeout:  timeoutDuration,
		WriteTimeout: timeoutDuration,
		IdleTimeout:  2 * timeoutDuration,
	}

	return &Server{
		server: srv,
	}, nil
}

// Start starts the HTTP server on the specified address
func (s *Server) Start(addr string) error {
	s.server.Addr = addr
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// logMiddleware adds logging to HTTP requests
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a custom response writer to capture status code
		rw := &responseWriter{w, http.StatusOK}

		// Process request
		next.ServeHTTP(rw, r)

		// Log request details
		duration := time.Since(start)
		logger := slog.With(
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)

		if rw.status >= 500 {
			logger.Error("Server error")
			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				hub.CaptureException(errors.New("Server error"))
			}
		} else if rw.status >= 400 {
			logger.Warn("Client error")
		} else {
			logger.Info("Request processed")
		}
	})
}

// responseWriter is a custom ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
