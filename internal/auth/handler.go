package auth

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/getsentry/sentry-go"
)

// AuthRequest represents the structure of incoming auth_callout request from NATS
type AuthRequest struct {
	ServerID string `json:"server_id"`
	ClientID int    `json:"client_id"`
	Subject  string `json:"subject"`
	Host     string `json:"host"`
	Tags     any    `json:"tags"`
	Name     string `json:"name"`
	Lang     string `json:"lang"`
	Version  string `json:"version"`
	User     string `json:"user"`     // GitLab username
	Password string `json:"password"` // GitLab Personal Access Token
}

// AuthResponse represents the structure of the response to NATS
type AuthResponse struct {
	OK          bool         `json:"ok"`
	Permissions *Permissions `json:"permissions,omitempty"`
}

// Permissions represents the NATS permissions for a user
type Permissions struct {
	Publish   *PermissionRules `json:"publish,omitempty"`
	Subscribe *PermissionRules `json:"subscribe,omitempty"`
}

// PermissionRules defines allow/deny rules for NATS operations
type PermissionRules struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// Handler handles the auth_callout requests
type Handler struct {
	gitlabClient *GitLabClient
}

// NewHandler creates a new auth handler
func NewHandler() *Handler {
	return &Handler{
		gitlabClient: NewGitLabClient(),
	}
}

// HandleAuth handles the auth_callout request from NATS
func (h *Handler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("handler", "auth")

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w)
		return
	}

	// Parse request JSON
	var authReq AuthRequest
	if err := json.Unmarshal(body, &authReq); err != nil {
		logger.Error("Failed to parse auth request", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w)
		return
	}

	logger = logger.With(
		"client_id", authReq.ClientID,
		"user", authReq.User,
		"client_name", authReq.Name,
	)

	// Validate request
	if authReq.User == "" || authReq.Password == "" {
		logger.Warn("Missing credentials in auth request")
		sendErrorResponse(w)
		return
	}

	// Verify GitLab token
	authentic, err := h.gitlabClient.VerifyToken(authReq.User, authReq.Password)
	if err != nil {
		logger.Error("Error verifying GitLab token", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w)
		return
	}

	if !authentic {
		logger.Info("Authentication failed", "reason", "invalid credentials")
		sendErrorResponse(w)
		return
	}

	// Authentication successful, provide permissions
	logger.Info("Authentication successful")
	permissions := &Permissions{
		Publish: &PermissionRules{
			Allow: []string{">"},
		},
		Subscribe: &PermissionRules{
			Allow: []string{">"},
		},
	}

	response := AuthResponse{
		OK:          true,
		Permissions: permissions,
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode response", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w)
		return
	}
}

// sendErrorResponse sends a failed authentication response
func sendErrorResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	response := AuthResponse{
		OK: false,
	}
	json.NewEncoder(w).Encode(response)
}
