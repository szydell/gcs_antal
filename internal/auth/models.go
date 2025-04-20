package auth

// NATSRequest represents the authentication request from NATS
type NATSRequest struct {
	NATS struct {
		ServerID    string      `json:"server_id"`
		ClientInfo  ClientInfo  `json:"client_info"`
		ConnectOpts ConnectOpts `json:"connect_opts"`
		ClientTLS   interface{} `json:"client_tls"`
	} `json:"nats"`
}

// ClientInfo represents client information in the NATS request
type ClientInfo struct {
	Host       string      `json:"host"`
	Port       int         `json:"port"`
	ID         int         `json:"id"`
	User       string      `json:"user"`
	Name       string      `json:"name"`
	Tags       interface{} `json:"tags"`
	Lang       string      `json:"lang"`
	Version    string      `json:"version"`
	Protocol   int         `json:"protocol"`
	Account    string      `json:"account"`
	JWT        string      `json:"jwt"`
	IssuerKey  string      `json:"issuer_key"`
	NameTag    string      `json:"name_tag"`
	Kind       int         `json:"kind"`
	ClientType int         `json:"client_type"`
	ClientIP   string      `json:"client_ip"`
}

// ConnectOpts represents connection options in the NATS request
type ConnectOpts struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"` // This will contain the GitLab PAT
	Name     string `json:"name"`
	Lang     string `json:"lang"`
	Version  string `json:"version"`
}

// NATSResponse represents the response structure for NATS auth_callout
type NATSResponse struct {
	OK          bool         `json:"ok"`
	Permissions *Permissions `json:"permissions,omitempty"`
}

// Permissions represents the NATS permissions for a user
type Permissions struct {
	Publish   *PermissionRules `json:"publish"`
	Subscribe *PermissionRules `json:"subscribe"`
}

// PermissionRules defines allow/deny rules for NATS operations
type PermissionRules struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}
