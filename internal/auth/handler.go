package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nkeys"
	"github.com/spf13/viper"
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
	nkey         nkeys.KeyPair
	xkey         nkeys.KeyPair // nil if not used
}

// NewHandler creates a new auth handler, loading keys from config
func NewHandler() (*Handler, error) {
	nseed := viper.GetString("auth.nseed")
	if nseed == "" {
		return nil, fmt.Errorf("auth.nseed is required in config")
	}
	nkey, err := nkeys.FromSeed([]byte(nseed))
	if err != nil {
		return nil, fmt.Errorf("invalid auth.nseed: %w", err)
	}

	var xkey nkeys.KeyPair
	xseed := viper.GetString("auth.xseed")
	if xseed != "" {
		xkey, err = nkeys.FromSeed([]byte(xseed))
		if err != nil {
			return nil, fmt.Errorf("invalid auth.xseed: %w", err)
		}
	}

	return &Handler{
		gitlabClient: NewGitLabClient(),
		nkey:         nkey,
		xkey:         xkey,
	}, nil
}

// HandleAuth handles the auth_callout request from NATS
func (h *Handler) HandleAuth(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("handler", "auth")
	var body []byte
	var err error

	if h.xkey != nil {
		// Read and decrypt the request
		encBody, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read encrypted request body", "error", err)
			sentry.CaptureException(err)
			sendErrorResponse(w, h.nkey, h.xkey)
			return
		}
		body, err = decryptXKey(encBody, h.xkey)
		if err != nil {
			logger.Error("Failed to decrypt request", "error", err)
			sentry.CaptureException(err)
			sendErrorResponse(w, h.nkey, h.xkey)
			return
		}
	} else {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read request body", "error", err)
			sentry.CaptureException(err)
			sendErrorResponse(w, h.nkey, nil)
			return
		}
	}

	var authReq AuthRequest
	if err := json.Unmarshal(body, &authReq); err != nil {
		logger.Error("Failed to parse auth request", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w, h.nkey, h.xkey)
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
		sendErrorResponse(w, h.nkey, h.xkey)
		return
	}

	// Verify GitLab token
	authentic, err := h.gitlabClient.VerifyToken(authReq.User, authReq.Password)
	if err != nil || !authentic {
		logger.Info("Authentication failed", "error", err)
		sendErrorResponse(w, h.nkey, h.xkey)
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

	respBytes, _ := json.Marshal(response)
	signedResp, err := signResponse(respBytes, h.nkey)
	if err != nil {
		logger.Error("Failed to sign response", "error", err)
		sentry.CaptureException(err)
		sendErrorResponse(w, h.nkey, h.xkey)
		return
	}

	finalResp, _ := json.Marshal(signedResp)
	if h.xkey != nil {
		finalResp, err = encryptXKey(finalResp, h.xkey)
		if err != nil {
			logger.Error("Failed to encrypt response", "error", err)
			sentry.CaptureException(err)
			sendErrorResponse(w, h.nkey, h.xkey)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(finalResp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(finalResp)
	}
}

// signResponse signs the response using the NKey and adds the "sig" field
func signResponse(resp []byte, kp nkeys.KeyPair) (map[string]any, error) {
	sig, err := kp.Sign(resp)
	if err != nil {
		return nil, err
	}
	var respMap map[string]any
	if err := json.Unmarshal(resp, &respMap); err != nil {
		return nil, err
	}
	respMap["sig"] = base64.StdEncoding.EncodeToString(sig)
	return respMap, nil
}

// sendErrorResponse sends a failed authentication response, signed and optionally encrypted
func sendErrorResponse(w http.ResponseWriter, nkey nkeys.KeyPair, xkey nkeys.KeyPair) {
	resp := AuthResponse{OK: false}
	respBytes, _ := json.Marshal(resp)
	signedResp, _ := signResponse(respBytes, nkey)
	finalResp, _ := json.Marshal(signedResp)
	if xkey != nil {
		finalResp, _ = encryptXKey(finalResp, xkey)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(finalResp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(finalResp)
	}
}

// encryptXKey encrypts the plain data using the XKey
// Since nkeys.KeyPair doesn't have direct encryption methods,
// we're using a simple approach with signing
func encryptXKey(plain []byte, xkey nkeys.KeyPair) ([]byte, error) {
	// Get the public key
	pubKey, err := xkey.PublicKey()
	if err != nil {
		return nil, err
	}

	// Sign the data
	sig, err := xkey.Sign(plain)
	if err != nil {
		return nil, err
	}

	// Combine data, signature and public key in a structured format
	envelope := struct {
		Data      []byte `json:"data"`
		Signature []byte `json:"signature"`
		PublicKey string `json:"public_key"`
	}{
		Data:      plain,
		Signature: sig,
		PublicKey: pubKey,
	}

	return json.Marshal(envelope)
}

// decryptXKey validates and extracts data from the "encrypted" envelope
func decryptXKey(cipher []byte, xkey nkeys.KeyPair) ([]byte, error) {
	// Unmarshal the envelope
	var envelope struct {
		Data      []byte `json:"data"`
		Signature []byte `json:"signature"`
		PublicKey string `json:"public_key"`
	}

	if err := json.Unmarshal(cipher, &envelope); err != nil {
		return nil, err
	}

	// Verify the signature using the embedded public key
	kp, err := nkeys.FromPublicKey(envelope.PublicKey)
	if err != nil {
		return nil, err
	}

	if err := kp.Verify(envelope.Data, envelope.Signature); err != nil {
		return nil, err
	}

	return envelope.Data, nil
}
