package auth

import (
	"crypto/rand" // Import crypto/rand
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// NATSClient handles NATS authentication requests
type NATSClient struct {
	nc            *nats.Conn
	issuerKeyPair nkeys.KeyPair
	xKeyPair      nkeys.KeyPair // May be nil if not using encryption
	gitlabClient  *GitLabClient
	logger        *slog.Logger
}

// NewNATSClient creates a new NATS client
func NewNATSClient(url, user, pass string, issuerSeed, xKeySeed string, gitlabClient *GitLabClient) (*NATSClient, error) {
	logger := slog.With("component", "nats_client")

	// Parse the issuer seed
	issuerKeyPair, err := nkeys.FromSeed([]byte(issuerSeed))
	if err != nil {
		return nil, fmt.Errorf("invalid issuer seed: %w", err)
	}

	// Parse the xKey seed if provided
	var xKeyPair nkeys.KeyPair
	if xKeySeed != "" {
		xKeyPair, err = nkeys.FromSeed([]byte(xKeySeed))
		if err != nil {
			return nil, fmt.Errorf("invalid xKey seed: %w", err)
		}
	}

	// Connect to NATS
	opts := []nats.Option{
		nats.ReconnectWait(5 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Warn("Disconnected from NATS", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("Reconnected to NATS", "server", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(nc *nats.Conn, s *nats.Subscription, err error) {
			logger.Error("NATS error", "error", err)
		}),
	}

	// Add authentication if provided
	if user != "" && pass != "" {
		opts = append(opts, nats.UserInfo(user, pass))
	}

	// Connect to NATS
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	logger.Info("Connected to NATS server", "url", url)

	return &NATSClient{
		nc:            nc,
		issuerKeyPair: issuerKeyPair,
		xKeyPair:      xKeyPair,
		gitlabClient:  gitlabClient,
		logger:        logger,
	}, nil
}

// Start starts listening for authentication requests
func (c *NATSClient) Start() error {
	// Subscribe to the auth_callout subject
	_, err := c.nc.Subscribe("$SYS.REQ.USER.AUTH", func(msg *nats.Msg) {
		c.handleAuthRequest(msg)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to auth requests: %w", err)
	}

	c.logger.Info("Started listening for authentication requests")
	return nil
}

// handleAuthRequest processes an authentication request from NATS
func (c *NATSClient) handleAuthRequest(msg *nats.Msg) {
	var request NATSRequest
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		c.logger.Error("Failed to unmarshal auth request", "error", err)
		c.sendFailureResponse(msg.Reply)
		return
	}

	// Extract credentials from the request
	username := request.NATS.ConnectOpts.Username
	token := request.NATS.ConnectOpts.Password

	c.logger.Info("Received auth request",
		"client_id", request.NATS.ClientInfo.ID,
		"username", username)

	// Verify GitLab token
	authentic, err := c.gitlabClient.VerifyToken(username, token)
	if err != nil {
		c.logger.Error("Error verifying GitLab token", "error", err)
		c.sendFailureResponse(msg.Reply)
		sentry.CaptureException(err)
		return
	}

	if !authentic {
		c.logger.Info("Authentication failed", "username", username)
		c.sendFailureResponse(msg.Reply)
		return
	}

	// Authentication successful, provide permissions
	c.logger.Info("Authentication successful", "username", username)

	// Prepare response
	response := NATSResponse{
		OK: true,
		Permissions: &Permissions{
			Publish: &PermissionRules{
				Allow: []string{"topic.>"},
				Deny:  []string{"private.>"},
			},
			Subscribe: &PermissionRules{
				Allow: []string{"topic.>"},
				Deny:  []string{"private.>"},
			},
		},
	}

	// Send response
	c.sendSuccessResponse(msg.Reply, response)
}

// sendSuccessResponse sends a successful authentication response
func (c *NATSClient) sendSuccessResponse(replySubject string, response NATSResponse) {
	// Marshal the response to JSON
	respBytes, err := json.Marshal(response)
	if err != nil {
		c.logger.Error("Failed to marshal response", "error", err)
		c.sendFailureResponse(replySubject)
		return
	}

	// Sign the response
	sig, err := c.issuerKeyPair.Sign(respBytes)
	if err != nil {
		c.logger.Error("Failed to sign response", "error", err)
		c.sendFailureResponse(replySubject)
		return
	}

	// Create signed response
	signedResp := map[string]any{
		"data": string(respBytes),
		"sig":  base64.StdEncoding.EncodeToString(sig),
	}

	finalResp, err := json.Marshal(signedResp)
	if err != nil {
		c.logger.Error("Failed to marshal signed response", "error", err)
		c.sendFailureResponse(replySubject)
		return
	}

	// Encrypt if xKey is available
	if c.xKeyPair != nil {
		finalResp, err = c.encryptResponse(finalResp)
		if err != nil {
			c.logger.Error("Failed to encrypt response", "error", err)
			c.sendFailureResponse(replySubject)
			return
		}
	}

	// Send the response
	if err := c.nc.Publish(replySubject, finalResp); err != nil {
		c.logger.Error("Failed to publish response", "error", err)
	}
}

// sendFailureResponse sends a failure response
func (c *NATSClient) sendFailureResponse(replySubject string) {
	response := NATSResponse{OK: false}

	respBytes, err := json.Marshal(response)
	if err != nil {
		c.logger.Error("Failed to marshal failure response", "error", err)
		return
	}

	// Sign the response
	sig, err := c.issuerKeyPair.Sign(respBytes)
	if err != nil {
		c.logger.Error("Failed to sign failure response", "error", err)
		return
	}

	// Create signed response
	signedResp := map[string]any{
		"data": string(respBytes),
		"sig":  base64.StdEncoding.EncodeToString(sig),
	}

	finalResp, err := json.Marshal(signedResp)
	if err != nil {
		c.logger.Error("Failed to marshal signed failure response", "error", err)
		return
	}

	// Encrypt if xKey is available
	if c.xKeyPair != nil {
		finalResp, err = c.encryptResponse(finalResp)
		if err != nil {
			c.logger.Error("Failed to encrypt failure response", "error", err)
			return
		}
	}

	// Send the response
	if err := c.nc.Publish(replySubject, finalResp); err != nil {
		c.logger.Error("Failed to publish failure response", "error", err)
	}
}

// encryptResponse encrypts the response using the xKey
func (c *NATSClient) encryptResponse(data []byte) ([]byte, error) {
	// In NATS auth_callout, we use the xKey to encrypt responses
	// The server already has our public key configured

	// Generate a 24-byte nonce using crypto/rand
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data using our xKey's private key (Seal method)
	// Convert data to string before passing to Seal
	cipher, err := c.xKeyPair.Seal(nonce, string(data)) // Convert data to string here
	if err != nil {
		return nil, fmt.Errorf("encryption failed using Seal: %w", err)
	}

	// Format the response according to NATS auth_callout expectations
	// The server will use our public xKey (which it has) to decrypt this message
	encResp := map[string]string{
		"nonce": base64.StdEncoding.EncodeToString(nonce),
		"data":  base64.StdEncoding.EncodeToString(cipher),
	}

	// Marshal the encrypted response structure to JSON
	respBytes, err := json.Marshal(encResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal encrypted response: %w", err)
	}

	return respBytes, nil
}

// Stop cleanly closes the NATS connection
func (c *NATSClient) Stop() {
	if c.nc != nil && !c.nc.IsClosed() {
		c.logger.Info("Closing NATS connection")
		c.nc.Close()
	}
}
