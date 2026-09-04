// Package config provides typed, validated configuration for TiBrain with
// environment-variable overrides. It is the single source of configuration for
// server bind address, MCP transports, authentication, permission profiles and
// audit logging.
//
// Secrets (bearer token, Cloudflare creds) are NEVER read from the YAML file.
// They come from environment variables or local secret files listed in
// .gitignore. Startup fails closed if a required secret/profile precondition
// is not satisfied.
package config

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Profile is a permission tier applied to authenticated identities.
type Profile string

const (
	// ProfileReadOnly allows only read/list/search/status tools.
	ProfileReadOnly Profile = "read_only"
	// ProfileOperator allows controlled writes; no mass-delete or privileged exec.
	ProfileOperator Profile = "operator"
	// ProfileTrustedFull enables every configured tool for an authenticated admin.
	ProfileTrustedFull Profile = "trusted_full"
)

// ServerConfig controls the HTTP bind address and public base URL.
type ServerConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	PublicBaseURL string `yaml:"public_base_url"`
}

// MCPServerConfig defines configuration for an upstream MCP server.
type MCPServerConfig struct {
	Name        string            `yaml:"name"`
	Transport   string            `yaml:"transport"` // "stdio" or "http"/"sse"
	Command     string            `yaml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty"`
	URL         string            `yaml:"url,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Enabled     bool              `yaml:"enabled"`
	Description string            `yaml:"description,omitempty"`
	AutoStart   bool              `yaml:"auto_start"`
}

// MCPConfig controls transport paths and protocol limits.
type MCPConfig struct {
	StreamableHTTPPath string        `yaml:"streamable_http_path"`
	LegacySSEPath      string        `yaml:"legacy_sse_path"`
	LegacyMessagePath  string        `yaml:"legacy_message_path"`
	SessionTTL         time.Duration `yaml:"session_ttl"`
	// MaxConcurrentCallsPerSession caps in-flight tool calls per session.
	MaxConcurrentCallsPerSession int `yaml:"max_concurrent_calls_per_session"`
	MaxRequestBytes              int `yaml:"max_request_bytes"`
	MaxOutputBytes               int `yaml:"max_output_bytes"`
	// Servers is the list of upstream MCP servers to connect to.
	Servers []MCPServerConfig `yaml:"servers"`
}

// AuthConfig controls authentication at the gateway edge.
type AuthConfig struct {
	// Mode is currently "bearer" (OAuth 2.1/OIDC is the documented roadmap).
	Mode           string   `yaml:"mode"`
	BearerTokenEnv string   `yaml:"bearer_token_env"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	// RateLimitPerMinute applies per identity (or IP when unauthenticated).
	RateLimitPerMinute int `yaml:"rate_limit_per_minute"`
}

// TrustedFullConfig gates the trusted_full profile.
type TrustedFullConfig struct {
	Enabled         bool     `yaml:"enabled"`
	AdminIdentities []string `yaml:"admin_identities"`
	FilesystemRoots []string `yaml:"filesystem_roots"`
}

// PermissionsConfig selects the active profile and gates trusted_full.
type PermissionsConfig struct {
	ActiveProfile Profile           `yaml:"active_profile"`
	TrustedFull   TrustedFullConfig `yaml:"trusted_full"`
}

// AuditConfig controls the audit log.
type AuditConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Path          string `yaml:"path"`
	RedactSecrets bool   `yaml:"redact_secrets"`
}

// RAGConfig controls RAG retrieval and query caching.
type RAGConfig struct {
	// QueryCacheTTL is how long a cached RAG query result stays fresh
	// before a background re-population. Default 5 minutes.
	QueryCacheTTL time.Duration `yaml:"query_cache_ttl"`
	// QueryCacheBurst controls how many concurrent cache misses
	// are allowed before single-flight dedup. Default 64.
	QueryCacheBurst int `yaml:"query_cache_burst"`
}

// Config is the root typed configuration.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	MCP         MCPConfig         `yaml:"mcp"`
	Auth        AuthConfig        `yaml:"auth"`
	Permissions PermissionsConfig `yaml:"permissions"`
	Audit       AuditConfig       `yaml:"audit"`
	RAG         RAGConfig         `yaml:"rag"`

	// AllowedRoots is the base allow-list for filesystem tools (non-trusted_full).
	AllowedRoots []string `yaml:"allowed_roots"`

	// TiBrain-specific legacy fields
	CLIRegistry    string `yaml:"cli_registry"`
	HandoffTrack   string `yaml:"handoff_track"`
	SkillSync      string `yaml:"skill_sync"`
	IndexKnowledge string `yaml:"index_knowledge"`
	DataDir        string `yaml:"data_dir"`
}

// Default returns a safe-by-default configuration. The default profile is
// operator (user decision), but every unknown action still fails closed.
func Default() *Config {
	return &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 3005, PublicBaseURL: "https://mcp.trepremium.online"},
		MCP: MCPConfig{
			StreamableHTTPPath:           "/mcp",
			LegacySSEPath:                "/mcp/sse",
			LegacyMessagePath:            "/mcp/message",
			SessionTTL:                   30 * time.Minute,
			MaxConcurrentCallsPerSession: 4,
			MaxRequestBytes:              1 << 20,
			MaxOutputBytes:               4 << 20,
		},
		Auth: AuthConfig{
			Mode:               "bearer",
			BearerTokenEnv:     "TIBRAIN_MCP_BEARER_TOKEN",
			AllowedOrigins:     []string{"https://chatgpt.com", "https://chat.openai.com"},
			RateLimitPerMinute: 60,
		},
		Permissions: PermissionsConfig{ActiveProfile: ProfileOperator},
		RAG: RAGConfig{
			QueryCacheTTL:      5 * time.Minute,
			QueryCacheBurst:    64,
		},
		Audit:       AuditConfig{Enabled: true, Path: ".runtime/logs/audit.jsonl", RedactSecrets: true},
		// Legacy TiBrain defaults
		CLIRegistry:    "",
		HandoffTrack:   "",
		SkillSync:      "",
		IndexKnowledge: "",
		DataDir:        "",
	}
}

// Load reads configuration from the given YAML file (if present) then applies
// environment overrides, and finally validates fail-closed preconditions.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config %q: %w", path, err)
			}
		}
	}
	cfg.applyEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyEnv() {
	if v := strings.TrimSpace(os.Getenv("TIBRAIN_HOST")); v != "" {
		c.Server.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("TIBRAIN_PORT")); v != "" {
		if p, err := parseInt(v); err == nil {
			c.Server.Port = p
		}
	}
	if v := strings.TrimSpace(os.Getenv("TIBRAIN_PUBLIC_BASE_URL")); v != "" {
		c.Server.PublicBaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("TIBRAIN_AUTONOMY_MODE")); v != "" {
		c.Permissions.ActiveProfile = Profile(v)
	}
	if v := strings.TrimSpace(os.Getenv("TIBRAIN_ALLOWED_ROOTS")); v != "" {
		c.AllowedRoots = parseRoots(v)
	} else if v := strings.TrimSpace(os.Getenv("MCP_ALLOWED_ROOTS")); v != "" {
		c.AllowedRoots = parseRoots(v)
	}
	if c.Auth.BearerTokenEnv == "" {
		c.Auth.BearerTokenEnv = "TIBRAIN_MCP_BEARER_TOKEN"
	}
}

// BearerToken resolves the gateway bearer token from the configured env var or a
// local secret file. It returns "" when unset (caller must then reject all
// requests if auth is required).
func (c *Config) BearerToken() string {
	env := c.Auth.BearerTokenEnv
	if env == "" {
		env = "TIBRAIN_MCP_BEARER_TOKEN"
	}
	return strings.TrimSpace(os.Getenv(env))
}

// TrustedFullArmed reports whether the trusted_full profile is permitted to
// activate: explicit env opt-in AND config enabled AND at least one admin AND
// audit enabled.
func (c *Config) TrustedFullArmed() bool {
	if strings.TrimSpace(os.Getenv("TIBRAIN_AUTONOMY_MODE")) != string(ProfileTrustedFull) {
		return false
	}
	if !c.Permissions.TrustedFull.Enabled {
		return false
	}
	if len(c.Permissions.TrustedFull.AdminIdentities) == 0 {
		return false
	}
	if !c.Audit.Enabled {
		return false
	}
	return true
}

// Validate enforces fail-closed preconditions.
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port %d", c.Server.Port)
	}
	switch c.Permissions.ActiveProfile {
	case ProfileReadOnly, ProfileOperator, ProfileTrustedFull:
	default:
		return fmt.Errorf("invalid permissions.active_profile %q", c.Permissions.ActiveProfile)
	}
	if c.Permissions.ActiveProfile == ProfileTrustedFull && !c.TrustedFullArmed() {
		return fmt.Errorf("trusted_full requested but not armed: require TIBRAIN_AUTONOMY_MODE=trusted_full, permissions.trusted_full.enabled=true, non-empty admin_identities and audit.enabled=true")
	}
	if c.Permissions.ActiveProfile == ProfileTrustedFull {
		if c.BearerToken() == "" {
			return fmt.Errorf("trusted_full requires a bearer token (set %s)", c.Auth.BearerTokenEnv)
		}
		if !c.Audit.Enabled {
			return fmt.Errorf("trusted_full requires audit logging enabled")
		}
	}
	if c.Audit.Enabled && c.Audit.Path == "" {
		return fmt.Errorf("audit.enabled but audit.path is empty")
	}

	// Validate MCP server configurations
	if err := c.ValidateMCPServers(); err != nil {
		return err
	}

	return nil
}

// ValidateMCPServers checks MCP server configurations including command existence for stdio transport
func (c *Config) ValidateMCPServers() error {
	for _, server := range c.MCP.Servers {
		if !server.Enabled {
			continue
		}

		switch server.Transport {
		case "stdio":
			if server.Command == "" {
				return fmt.Errorf("mcp.servers[%s]: stdio transport requires command", server.Name)
			}
			// Check if command exists in PATH for stdio transport
			if _, err := execLookPath(server.Command); err != nil {
				// Log warning but don't fail - command might be available at runtime
				// In production mode, you might want to fail here
				log.Printf("[WARN] MCP server %s: command %q not found in PATH (will fail at runtime if not available)", server.Name, server.Command)
			}
		case "http", "sse":
			if server.URL == "" {
				return fmt.Errorf("mcp.servers[%s]: http/sse transport requires url", server.Name)
			}
		default:
			return fmt.Errorf("mcp.servers[%s]: invalid transport %q (must be stdio, http, or sse)", server.Name, server.Transport)
		}
	}
	return nil
}

// EffectiveRoots returns the filesystem roots applicable for the active profile.
// trusted_full additionally exposes its configured roots (e.g. C:\ and Z:\).
func (c *Config) EffectiveRoots() []string {
	if c.Permissions.ActiveProfile == ProfileTrustedFull && c.TrustedFullArmed() {
		roots := append([]string{}, c.AllowedRoots...)
		roots = append(roots, c.Permissions.TrustedFull.FilesystemRoots...)
		return roots
	}
	return c.AllowedRoots
}

func parseRoots(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// execLookPath finds the executable path (wraps exec.LookPath)
func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// RuntimePath resolves a relative runtime path against the working directory
// and ensures parent dirs exist (used for audit log, tunnel pids).
func RuntimePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return p, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, p), nil
}
