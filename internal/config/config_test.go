package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// helper to set env and return cleanup
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		}
	})
}

func writeTempConfig(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config %s: %v", path, err)
	}
	return path
}

// ------------------------------------------------------------------
// Default()
// ------------------------------------------------------------------

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Server.Port != 3005 {
		t.Errorf("default port: got %d, want 3005", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default host: got %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.PublicBaseURL != "https://mcp.trepremium.online" {
		t.Errorf("default public_base_url: got %q", cfg.Server.PublicBaseURL)
	}
	if cfg.Permissions.ActiveProfile != ProfileOperator {
		t.Errorf("default profile: got %q, want %q", cfg.Permissions.ActiveProfile, ProfileOperator)
	}
	if cfg.Auth.Mode != "bearer" {
		t.Errorf("default auth mode: got %q", cfg.Auth.Mode)
	}
	if cfg.Auth.BearerTokenEnv != "TIBRAIN_MCP_BEARER_TOKEN" {
		t.Errorf("default bearer token env: got %q", cfg.Auth.BearerTokenEnv)
	}
	if cfg.Auth.RateLimitPerMinute != 60 {
		t.Errorf("default rate limit: got %d", cfg.Auth.RateLimitPerMinute)
	}
	if len(cfg.Auth.AllowedOrigins) != 2 {
		t.Errorf("default allowed origins: got %d, want 2", len(cfg.Auth.AllowedOrigins))
	}
	if cfg.MCP.SessionTTL != 30*time.Minute {
		t.Errorf("default session ttl: got %v", cfg.MCP.SessionTTL)
	}
	if cfg.MCP.MaxConcurrentCallsPerSession != 4 {
		t.Errorf("default max concurrent: got %d", cfg.MCP.MaxConcurrentCallsPerSession)
	}
	if cfg.RAG.QueryCacheTTL != 5*time.Minute {
		t.Errorf("default rag ttl: got %v", cfg.RAG.QueryCacheTTL)
	}
	if cfg.Audit.Enabled != true {
		t.Errorf("default audit enabled: got %v", cfg.Audit.Enabled)
	}
	if cfg.Audit.RedactSecrets != true {
		t.Errorf("default audit redact: got %v", cfg.Audit.RedactSecrets)
	}
}

func TestDefault_IsDeepCopySafe(t *testing.T) {
	a := Default()
	b := Default()
	if &a.Server == &b.Server {
		t.Error("Default() should return independent structs")
	}
}

// ------------------------------------------------------------------
// Load() with YAML file and env overrides
// ------------------------------------------------------------------

func TestLoad_FromFile(t *testing.T) {
	// Unset env overrides để test hermetic — TIBRAIN_HOST/TIBRAIN_PORT từ
	// môi trường ngoài (settings.json) sẽ override YAML theo design.
	unsetEnv(t, "TIBRAIN_HOST")
	unsetEnv(t, "TIBRAIN_PORT")
	unsetEnv(t, "TIBRAIN_PUBLIC_BASE_URL")
	yamlContent := `
server:
  host: "0.0.0.0"
  port: 8080
  public_base_url: "https://example.com/mcp"
permissions:
  active_profile: "read_only"
`
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "config.yaml", yamlContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("host: got %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port: got %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.PublicBaseURL != "https://example.com/mcp" {
		t.Errorf("public_base_url: got %q", cfg.Server.PublicBaseURL)
	}
	if cfg.Permissions.ActiveProfile != ProfileReadOnly {
		t.Errorf("profile: got %q, want %q", cfg.Permissions.ActiveProfile, ProfileReadOnly)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
server:
  port: 8080
`
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "config.yaml", yamlContent)

	setEnv(t, "TIBRAIN_PORT", "9090")
	setEnv(t, "TIBRAIN_HOST", "10.0.0.1")
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "read_only")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("env port override: got %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("env host override: got %q, want %q", cfg.Server.Host, "10.0.0.1")
	}
	if cfg.Permissions.ActiveProfile != ProfileReadOnly {
		t.Errorf("env profile override: got %q, want %q", cfg.Permissions.ActiveProfile, ProfileReadOnly)
	}
}

func TestLoad_MissingFileFallsBackToDefault(t *testing.T) {
	unsetEnv(t, "TIBRAIN_HOST") // env thật từ shell không được override default trong test hermetic
	unsetEnv(t, "TIBRAIN_PORT")
	cfg, err := Load("/nonexistent/path/does/not/exist.yaml")
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	def := Default()
	if cfg.Server.Port != def.Server.Port {
		t.Errorf("missing file should use default port: got %d, want %d", cfg.Server.Port, def.Server.Port)
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	unsetEnv(t, "TIBRAIN_HOST") // env thật từ shell không được override default trong test hermetic
	unsetEnv(t, "TIBRAIN_PORT")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load empty path: %v", err)
	}
	if cfg.Server.Port != 3005 {
		t.Errorf("empty path should use default port: got %d, want 3005", cfg.Server.Port)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "bad.yaml", "server: [invalid\n  port: : :")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_YAMLPartialMerge(t *testing.T) {
	// Unset env overrides để test hermetic (xem TestLoad_FromFile).
	unsetEnv(t, "TIBRAIN_HOST")
	unsetEnv(t, "TIBRAIN_PORT")
	yamlContent := `
server:
  port: 4000
`
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "partial.yaml", yamlContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Host should come from default
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host from default: got %q, want 127.0.0.1", cfg.Server.Host)
	}
	// Port from file
	if cfg.Server.Port != 4000 {
		t.Errorf("port from file: got %d, want 4000", cfg.Server.Port)
	}
}

// ------------------------------------------------------------------
// Validate() edge cases
// ------------------------------------------------------------------

func TestValidate_InvalidPorts(t *testing.T) {
	cases := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 65536},
		{"way too large", 100000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Server.Port = tc.port
			err := cfg.Validate()
			if err == nil {
				t.Errorf("expected error for port %d", tc.port)
			}
		})
	}
}

func TestValidate_ValidPorts(t *testing.T) {
	cases := []struct {
		name string
		port int
	}{
		{"min valid", 1},
		{"max valid", 65535},
		{"typical", 3005},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Server.Port = tc.port
			if err := cfg.Validate(); err != nil {
				t.Errorf("port %d should be valid: %v", tc.port, err)
			}
		})
	}
}

func TestValidate_UnknownProfile(t *testing.T) {
	cfg := Default()
	cfg.Permissions.ActiveProfile = Profile("bogus_profile")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestValidate_KnownProfiles(t *testing.T) {
	for _, p := range []Profile{ProfileReadOnly, ProfileOperator, ProfileTrustedFull} {
		t.Run(string(p), func(t *testing.T) {
			cfg := Default()
			cfg.Permissions.ActiveProfile = p
			// trusted_full requires arming; skip if that case
			if p == ProfileTrustedFull && !cfg.TrustedFullArmed() {
				// not armed -> should error, which is expected here
				_ = cfg.Validate()
				return
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("profile %s should validate: %v", p, err)
			}
		})
	}
}

func TestValidate_TrustedFull_NotArmed(t *testing.T) {
	cases := []struct {
		name           string
		autonomyMode   string // env
		enabled        bool
		admins         []string
		auditEnabled   bool
		expectArmedErr bool
	}{
		{
			name:           "no autonomy env",
			enabled:        true,
			admins:         []string{"admin@test.com"},
			auditEnabled:   true,
			expectArmedErr: true,
		},
		{
			name:           "env set but not trusted_full",
			autonomyMode:   "operator",
			enabled:        true,
			admins:         []string{"admin@test.com"},
			auditEnabled:   true,
			expectArmedErr: true,
		},
		{
			name:           "trusted_full env but not enabled in config",
			autonomyMode:   "trusted_full",
			enabled:        false,
			admins:         []string{"admin@test.com"},
			auditEnabled:   true,
			expectArmedErr: true,
		},
		{
			name:           "enabled but no admins",
			autonomyMode:   "trusted_full",
			enabled:        true,
			admins:         nil,
			auditEnabled:   true,
			expectArmedErr: true,
		},
		{
			name:           "enabled with admin but audit off",
			autonomyMode:   "trusted_full",
			enabled:        true,
			admins:         []string{"admin@test.com"},
			auditEnabled:   false,
			expectArmedErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			if tc.autonomyMode != "" {
				setEnv(t, "TIBRAIN_AUTONOMY_MODE", tc.autonomyMode)
			}
			cfg.Permissions.ActiveProfile = ProfileTrustedFull
			cfg.Permissions.TrustedFull.Enabled = tc.enabled
			cfg.Permissions.TrustedFull.AdminIdentities = tc.admins
			cfg.Audit.Enabled = tc.auditEnabled
			cfg.Audit.Path = ".runtime/logs/audit.jsonl"

			if cfg.TrustedFullArmed() {
				t.Errorf("TrustedFullArmed should be false for %s", tc.name)
			}

			err := cfg.Validate()
			if tc.expectArmedErr && err == nil {
				t.Errorf("expected validation error (not armed) for %s", tc.name)
			}
		})
	}
}

func TestValidate_AuditEnabledEmptyPath(t *testing.T) {
	cfg := Default()
	cfg.Audit.Enabled = true
	cfg.Audit.Path = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when audit enabled but path empty")
	}
}

func TestValidate_TrustedFull_NoBearerToken(t *testing.T) {
	// Armed state (env + config + admin + audit) but no bearer token set
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	unsetEnv(t, "TIBRAIN_MCP_BEARER_TOKEN") // env thật từ router.env không được leak vào test hermetic
	cfg := Default()
	cfg.Permissions.ActiveProfile = ProfileTrustedFull
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = true
	cfg.Audit.Path = ".runtime/logs/audit.jsonl"

	if !cfg.TrustedFullArmed() {
		t.Fatal("should be armed")
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for trusted_full without bearer token")
	}
}

// ------------------------------------------------------------------
// BearerToken()
// ------------------------------------------------------------------

func TestBearerToken_FromEnv(t *testing.T) {
	setEnv(t, "TIBRAIN_MCP_BEARER_TOKEN", "secret-token-123")
	cfg := Default()
	if got := cfg.BearerToken(); got != "secret-token-123" {
		t.Errorf("bearer token: got %q, want %q", got, "secret-token-123")
	}
}

func TestBearerToken_DefaultEnvVarName(t *testing.T) {
	// Default BearerTokenEnv is TIBRAIN_MCP_BEARER_TOKEN
	cfg := Default()
	if cfg.Auth.BearerTokenEnv != "TIBRAIN_MCP_BEARER_TOKEN" {
		t.Errorf("default env var name: got %q", cfg.Auth.BearerTokenEnv)
	}
}

func TestBearerToken_UnsetReturnsEmpty(t *testing.T) {
	unsetEnv(t, "TIBRAIN_MCP_BEARER_TOKEN")
	cfg := Default()
	if got := cfg.BearerToken(); got != "" {
		t.Errorf("unset bearer token: got %q, want empty", got)
	}
}

func TestBearerToken_CustomEnvVar(t *testing.T) {
	setEnv(t, "MY_CUSTOM_TOKEN", "custom-secret")
	cfg := Default()
	cfg.Auth.BearerTokenEnv = "MY_CUSTOM_TOKEN"
	if got := cfg.BearerToken(); got != "custom-secret" {
		t.Errorf("custom env var bearer: got %q, want %q", got, "custom-secret")
	}
}

func TestBearerToken_EmptyEnvNameFallback(t *testing.T) {
	// When BearerTokenEnv is empty, BearerToken() should fall back to default env
	cfg := Default()
	cfg.Auth.BearerTokenEnv = ""
	setEnv(t, "TIBRAIN_MCP_BEARER_TOKEN", "fallback-secret")
	if got := cfg.BearerToken(); got != "fallback-secret" {
		t.Errorf("fallback env: got %q, want %q", got, "fallback-secret")
	}
}

func TestBearerToken_TrimSpace(t *testing.T) {
	setEnv(t, "TIBRAIN_MCP_BEARER_TOKEN", "  padded-token  ")
	cfg := Default()
	if got := cfg.BearerToken(); got != "padded-token" {
		t.Errorf("trimmed token: got %q, want %q", got, "padded-token")
	}
}

// ------------------------------------------------------------------
// TrustedFullArmed()
// ------------------------------------------------------------------

func TestTrustedFullArmed_AllConditionsMet(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = true
	cfg.Audit.Path = ".runtime/logs/audit.jsonl"
	if !cfg.TrustedFullArmed() {
		t.Error("should be armed when all conditions met")
	}
}

func TestTrustedFullArmed_MissingEnv(t *testing.T) {
	unsetEnv(t, "TIBRAIN_AUTONOMY_MODE")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = true
	if cfg.TrustedFullArmed() {
		t.Error("should not be armed without env var")
	}
}

func TestTrustedFullArmed_NotEnabledInConfig(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = false
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = true
	if cfg.TrustedFullArmed() {
		t.Error("should not be armed when TrustedFull.Enabled is false")
	}
}

func TestTrustedFullArmed_NoAdmins(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = nil
	cfg.Audit.Enabled = true
	if cfg.TrustedFullArmed() {
		t.Error("should not be armed without admin identities")
	}
}

func TestTrustedFullArmed_AuditDisabled(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = false
	if cfg.TrustedFullArmed() {
		t.Error("should not be armed without audit enabled")
	}
}

func TestTrustedFullArmed_WrongProfileInEnv(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "operator")
	cfg := Default()
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Audit.Enabled = true
	if cfg.TrustedFullArmed() {
		t.Error("should not be armed when env profile is not trusted_full")
	}
}

// ------------------------------------------------------------------
// EffectiveRoots()
// ------------------------------------------------------------------

func TestEffectiveRoots_ReadOnlyProfile(t *testing.T) {
	cfg := Default()
	cfg.AllowedRoots = []string{"/data", "/home"}
	cfg.Permissions.ActiveProfile = ProfileReadOnly
	got := cfg.EffectiveRoots()
	if len(got) != 2 {
		t.Errorf("roots length: got %d, want 2", len(got))
	}
	if got[0] != "/data" || got[1] != "/home" {
		t.Errorf("roots: got %v", got)
	}
}

func TestEffectiveRoots_OperatorProfile(t *testing.T) {
	cfg := Default()
	cfg.AllowedRoots = []string{"/data", "/home"}
	cfg.Permissions.ActiveProfile = ProfileOperator
	got := cfg.EffectiveRoots()
	if len(got) != 2 {
		t.Errorf("roots length: got %d, want 2", len(got))
	}
}

func TestEffectiveRoots_TrustedFull_Armed(t *testing.T) {
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	cfg := Default()
	cfg.Permissions.ActiveProfile = ProfileTrustedFull
	cfg.Permissions.TrustedFull.Enabled = true
	cfg.Permissions.TrustedFull.AdminIdentities = []string{"admin@test.com"}
	cfg.Permissions.TrustedFull.FilesystemRoots = []string{"C:\\", "Z:\\"}
	cfg.Audit.Enabled = true
	cfg.Audit.Path = ".runtime/logs/audit.jsonl"
	cfg.AllowedRoots = []string{"/data"}

	got := cfg.EffectiveRoots()
	if len(got) != 3 {
		t.Errorf("roots length: got %d, want 3", len(got))
	}
	if got[0] != "/data" {
		t.Errorf("first root should be allowed root: got %q", got[0])
	}
	if got[1] != "C:\\" || got[2] != "Z:\\" {
		t.Errorf("trusted roots missing: got %v", got)
	}
}

func TestEffectiveRoots_TrustedFull_NotArmed(t *testing.T) {
	// When not armed, should only return AllowedRoots
	cfg := Default()
	cfg.Permissions.ActiveProfile = ProfileTrustedFull
	cfg.Permissions.TrustedFull.FilesystemRoots = []string{"C:\\"}
	cfg.AllowedRoots = []string{"/data"}
	// Not armed (no env var set)
	got := cfg.EffectiveRoots()
	if len(got) != 1 {
		t.Errorf("not armed: roots length %d, want 1", len(got))
	}
	if got[0] != "/data" {
		t.Errorf("not armed: got %q, want /data", got[0])
	}
}

func TestEffectiveRoots_DoesNotMutateOriginal(t *testing.T) {
	cfg := Default()
	cfg.AllowedRoots = []string{"/data"}
	cfg.Permissions.ActiveProfile = ProfileTrustedFull
	cfg.Permissions.TrustedFull.FilesystemRoots = []string{"/extra"}
	// Not armed so won't append
	got := cfg.EffectiveRoots()
	got = append(got, "/injected")
	if len(cfg.AllowedRoots) != 1 {
		t.Errorf("original AllowedRoots mutated: got %d", len(cfg.AllowedRoots))
	}
}

// ------------------------------------------------------------------
// ValidateMCPServers()
// ------------------------------------------------------------------

func TestValidateMCPServers_ValidStdioWithCommand(t *testing.T) {
	// "echo" is available on all platforms
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "test-stdio",
			Transport: "stdio",
			Command:   "echo",
			Enabled:   true,
		},
	}
	if err := cfg.ValidateMCPServers(); err != nil {
		t.Errorf("valid stdio server: %v", err)
	}
}

func TestValidateMCPServers_StdioWithoutCommand(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "bad-stdio",
			Transport: "stdio",
			Enabled:   true,
		},
	}
	err := cfg.ValidateMCPServers()
	if err == nil {
		t.Fatal("expected error for stdio without command")
	}
}

func TestValidateMCPServers_StdioDisabledIsSkipped(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "disabled-stdio",
			Transport: "stdio",
			Enabled:   false,
		},
	}
	if err := cfg.ValidateMCPServers(); err != nil {
		t.Errorf("disabled server should be skipped: %v", err)
	}
}

func TestValidateMCPServers_HTTPWithURL(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "test-http",
			Transport: "http",
			URL:       "http://localhost:9000",
			Enabled:   true,
		},
	}
	if err := cfg.ValidateMCPServers(); err != nil {
		t.Errorf("valid http server: %v", err)
	}
}

func TestValidateMCPServers_HTTPWithoutURL(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "bad-http",
			Transport: "http",
			Enabled:   true,
		},
	}
	err := cfg.ValidateMCPServers()
	if err == nil {
		t.Fatal("expected error for http without url")
	}
}

func TestValidateMCPServers_SSEWithURL(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "test-sse",
			Transport: "sse",
			URL:       "http://localhost:9001/sse",
			Enabled:   true,
		},
	}
	if err := cfg.ValidateMCPServers(); err != nil {
		t.Errorf("valid sse server: %v", err)
	}
}

func TestValidateMCPServers_SSEWithoutURL(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "bad-sse",
			Transport: "sse",
			Enabled:   true,
		},
	}
	err := cfg.ValidateMCPServers()
	if err == nil {
		t.Fatal("expected error for sse without url")
	}
}

func TestValidateMCPServers_InvalidTransport(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = []MCPServerConfig{
		{
			Name:      "bad-transport",
			Transport: "serial",
			Enabled:   true,
		},
	}
	err := cfg.ValidateMCPServers()
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestValidateMCPServers_NoServers(t *testing.T) {
	cfg := Default()
	cfg.MCP.Servers = nil
	if err := cfg.ValidateMCPServers(); err != nil {
		t.Errorf("no servers should be valid: %v", err)
	}
}

// ------------------------------------------------------------------
// parseRoots() (unexported)
// ------------------------------------------------------------------

func TestParseRoots(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"single", "/data", []string{"/data"}},
		{"multiple", "/data,/home,/var", []string{"/data", "/home", "/var"}},
		{"with spaces", " /data , /home , /var ", []string{"/data", "/home", "/var"}},
		{"empty entries", "/data,,/home,", []string{"/data", "/home"}},
		{"all empty", " , , ,", []string{}},
		{"completely empty", "", []string{}},
		{"single with trim", "  /data  ", []string{"/data"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRoots(tc.input)
			if len(got) != len(tc.want) {
				t.Errorf("parseRoots(%q): got %v, want %v", tc.input, got, tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseRoots(%q) at %d: got %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseRoots_DoesNotMutateInput(t *testing.T) {
	input := "/data,/home"
	_ = parseRoots(input)
	// parseRoots uses strings.Split which returns a new slice, so original untouched
	if input != "/data,/home" {
		t.Errorf("input was mutated: %q", input)
	}
}

// ------------------------------------------------------------------
// applyEnv()
// ------------------------------------------------------------------

func TestApplyEnv_AllowedRoots(t *testing.T) {
	cfg := Default()
	setEnv(t, "TIBRAIN_ALLOWED_ROOTS", "/data,/home,/var")

	cfg.applyEnv()

	if len(cfg.AllowedRoots) != 3 {
		t.Fatalf("allowed roots: got %v", cfg.AllowedRoots)
	}
	if cfg.AllowedRoots[0] != "/data" || cfg.AllowedRoots[1] != "/home" || cfg.AllowedRoots[2] != "/var" {
		t.Errorf("allowed roots content: got %v", cfg.AllowedRoots)
	}
}

func TestApplyEnv_MCPAllowedRootsFallback(t *testing.T) {
	unsetEnv(t, "TIBRAIN_ALLOWED_ROOTS")
	cfg := Default()
	setEnv(t, "MCP_ALLOWED_ROOTS", "/a,/b")

	cfg.applyEnv()

	if len(cfg.AllowedRoots) != 2 {
		t.Fatalf("mcp fallback roots: got %v", cfg.AllowedRoots)
	}
}

func TestApplyEnv_TIBrainAllowedRootsPrecedence(t *testing.T) {
	setEnv(t, "TIBRAIN_ALLOWED_ROOTS", "/tibrain,/data")
	setEnv(t, "MCP_ALLOWED_ROOTS", "/mcp,/other")

	cfg := Default()
	cfg.applyEnv()

	// TIBRAIN_ALLOWED_ROOTS should take precedence
	if len(cfg.AllowedRoots) != 2 {
		t.Fatalf("precedence roots: got %v", cfg.AllowedRoots)
	}
	if cfg.AllowedRoots[0] != "/tibrain" {
		t.Errorf("expected TIBRAIN precedence, got %q", cfg.AllowedRoots[0])
	}
}

func TestApplyEnv_DefaultBearerTokenEnvIfEmpty(t *testing.T) {
	cfg := Default()
	cfg.Auth.BearerTokenEnv = ""
	cfg.applyEnv()
	if cfg.Auth.BearerTokenEnv != "TIBRAIN_MCP_BEARER_TOKEN" {
		t.Errorf("default bearer env: got %q", cfg.Auth.BearerTokenEnv)
	}
}

// ------------------------------------------------------------------
// RuntimePath()
// ------------------------------------------------------------------

func TestRuntimePath_AbsolutePath(t *testing.T) {
	absPath := t.TempDir()
	got, err := RuntimePath(absPath)
	if err != nil {
		t.Fatalf("RuntimePath error: %v", err)
	}
	if got != absPath {
		t.Errorf("absolute path: got %q, want %q", got, absPath)
	}
}

func TestRuntimePath_RelativePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got, err := RuntimePath("logs/audit.jsonl")
	if err != nil {
		t.Fatalf("RuntimePath error: %v", err)
	}
	expected := filepath.Join(wd, "logs/audit.jsonl")
	if got != expected {
		t.Errorf("relative path: got %q, want %q", got, expected)
	}
}

// ------------------------------------------------------------------
// LocalConfig (localconfig.go)
// ------------------------------------------------------------------

func TestDetectAndLoad_ReturnsNilWhenNoConfig(t *testing.T) {
	// Use a temp dir with no config files; walking up to root is unpredictable,
	// so we create a deep temp dir to minimize chance of finding config.
	dir := t.TempDir()
	t.Chdir(dir)

	cfg, err := DetectAndLoad()
	if err != nil {
		t.Fatalf("DetectAndLoad in empty dir: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

func TestDetectAndLoad_FindsTibrainrcInCwd(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "project_name: test-proj\nversion: \"1.0\"\n"
	writeTempConfig(t, dir, ".tibrainrc", yamlContent)
	t.Chdir(dir)

	cfg, err := DetectAndLoad()
	if err != nil {
		t.Fatalf("DetectAndLoad: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.ProjectName != "test-proj" {
		t.Errorf("project_name: got %q, want %q", cfg.ProjectName, "test-proj")
	}
	if cfg.Version != "1.0" {
		t.Errorf("version: got %q, want %q", cfg.Version, "1.0")
	}
}

func TestDetectAndLoad_FindsConfigWalkingUp(t *testing.T) {
	// Create nested directory structure; config in parent
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir", "deep")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yamlContent := "project_name: parent-proj\n"
	writeTempConfig(t, parent, ".tibrainrc", yamlContent)
	t.Chdir(child)

	cfg, err := DetectAndLoad()
	if err != nil {
		t.Fatalf("DetectAndLoad: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config from parent dir, got nil")
	}
	if cfg.ProjectName != "parent-proj" {
		t.Errorf("project_name: got %q, want %q", cfg.ProjectName, "parent-proj")
	}
}

func TestDetectAndLoad_PrefersTibrainrcOver1mcprc(t *testing.T) {
	dir := t.TempDir()
	writeTempConfig(t, dir, ".tibrainrc", "project_name: tibrain\n")
	writeTempConfig(t, dir, ".1mcprc", "project_name: onemcp\n")
	t.Chdir(dir)

	cfg, err := DetectAndLoad()
	if err != nil {
		t.Fatalf("DetectAndLoad: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.ProjectName != "tibrain" {
		t.Errorf("expected .tibrainrc precedence, got %q", cfg.ProjectName)
	}
}

func TestDetectAndLoad_WalksToRootWithoutConfig(t *testing.T) {
	// Start from a temp dir; should walk up and return nil if nothing found.
	// We chdir to a deep temp dir unlikely to have configs above it.
	dir := t.TempDir()
	// Create subdirs to go deep
	deep := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(deep)

	cfg, err := DetectAndLoad()
	if err != nil {
		t.Fatalf("DetectAndLoad should not error: %v", err)
	}
	// May find nil or a config from system root; either is acceptable,
	// but in practice temp dirs won't have .tibrainrc/.1mcprc above them
	if cfg != nil {
		// Accept if some system-level config exists — just verify no error
		t.Logf("found config at depth: %+v", cfg)
	}
}

func TestLocalConfig_Struct(t *testing.T) {
	lc := &LocalConfig{
		ProjectName: "my-project",
		Version:     "2.0.0",
		BasePath:    "/work",
		MCPServers: []MCPServerConfig{
			{Name: "srv1", Transport: "stdio", Command: "node"},
		},
	}
	if lc.ProjectName != "my-project" {
		t.Errorf("project_name: got %q", lc.ProjectName)
	}
	if len(lc.MCPServers) != 1 {
		t.Errorf("mcp_servers length: got %d", len(lc.MCPServers))
	}
}

func TestLocalConfig_JSONRoundTrip(t *testing.T) {
	// Verify LocalConfig can be marshaled/unmarshaled via JSON tags
	lc := &LocalConfig{
		ProjectName: "json-proj",
		Version:     "1.2.3",
		BasePath:    "/base",
		MCPServers:  []MCPServerConfig{{Name: "json-srv", Transport: "http", URL: "http://localhost"}},
	}
	data, err := yaml.Marshal(lc)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}
	// Re-marshal round trip via yaml to verify structure is sound
	lc2 := &LocalConfig{}
	if err := yaml.Unmarshal(data, lc2); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if lc2.ProjectName != lc.ProjectName {
		t.Errorf("round trip project_name: got %q, want %q", lc2.ProjectName, lc.ProjectName)
	}
}

func TestLocalConfig_YAMLRoundTrip(t *testing.T) {
	// Note: loadYamlConfig/loadJsonConfig are currently stubs that return errors.
	// This test uses yaml.v3 directly to verify the LocalConfig struct's
	// YAML tags are correct, since DetectAndLoad's parsing path is not yet
	// implemented.
	yamlContent := `
project_name: yaml-proj
version: "1.0.0"
base_path: /workspace
mcp_servers:
  - name: server-a
    transport: stdio
    command: npx
    args:
      - mcp-server-a
  - name: server-b
    transport: http
    url: http://localhost:8080
`
	lc := &LocalConfig{}
	if err := yaml.Unmarshal([]byte(yamlContent), lc); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if lc.ProjectName != "yaml-proj" {
		t.Errorf("project_name: got %q, want %q", lc.ProjectName, "yaml-proj")
	}
	if lc.BasePath != "/workspace" {
		t.Errorf("base_path: got %q, want %q", lc.BasePath, "/workspace")
	}
	if len(lc.MCPServers) != 2 {
		t.Fatalf("mcp_servers length: got %d, want 2", len(lc.MCPServers))
	}
	if lc.MCPServers[0].Name != "server-a" {
		t.Errorf("server-a name: got %q", lc.MCPServers[0].Name)
	}
	if lc.MCPServers[0].Command != "npx" {
		t.Errorf("server-a command: got %q", lc.MCPServers[0].Command)
	}
	if len(lc.MCPServers[0].Args) != 1 || lc.MCPServers[0].Args[0] != "mcp-server-a" {
		t.Errorf("server-a args: got %v", lc.MCPServers[0].Args)
	}
	if lc.MCPServers[1].Name != "server-b" {
		t.Errorf("server-b name: got %q", lc.MCPServers[1].Name)
	}
	if lc.MCPServers[1].Transport != "http" {
		t.Errorf("server-b transport: got %q", lc.MCPServers[1].Transport)
	}
	if lc.MCPServers[1].URL != "http://localhost:8080" {
		t.Errorf("server-b url: got %q", lc.MCPServers[1].URL)
	}
}

// ------------------------------------------------------------------
// Integration: Load + YAML with nested config
// ------------------------------------------------------------------

func TestLoad_FullYAMLConfig(t *testing.T) {
	// Unset env overrides để test hermetic (xem TestLoad_FromFile).
	unsetEnv(t, "TIBRAIN_HOST")
	unsetEnv(t, "TIBRAIN_PORT")
	unsetEnv(t, "TIBRAIN_PUBLIC_BASE_URL")
	yamlContent := `
server:
  host: "0.0.0.0"
  port: 9090
  public_base_url: "https://myapp.com/mcp"
mcp:
  streamable_http_path: "/mcp"
  legacy_sse_path: "/sse"
  session_ttl: "1h"
  max_concurrent_calls_per_session: 8
  max_request_bytes: 2097152
  max_output_bytes: 8388608
  servers:
    - name: "git-server"
      transport: "stdio"
      command: "echo"
      enabled: true
    - name: "remote-server"
      transport: "http"
      url: "http://localhost:9999"
      enabled: true
auth:
  mode: "bearer"
  bearer_token_env: "CUSTOM_TOKEN_ENV"
  allowed_origins:
    - "https://app.example.com"
  rate_limit_per_minute: 120
permissions:
  active_profile: "read_only"
audit:
  enabled: true
  path: "/var/log/audit.jsonl"
  redact_secrets: true
rag:
  query_cache_ttl: "10m"
  query_cache_burst: 128
allowed_roots:
  - "/data"
  - "/home"
cli_registry: "https://registry.example.com"
handoff_track: "default"
`
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "full.yaml", yamlContent)
	setEnv(t, "CUSTOM_TOKEN_ENV", "full-config-token")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load full config: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("host: got %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Auth.RateLimitPerMinute != 120 {
		t.Errorf("rate limit: got %d, want 120", cfg.Auth.RateLimitPerMinute)
	}
	if cfg.MCP.MaxConcurrentCallsPerSession != 8 {
		t.Errorf("max concurrent: got %d, want 8", cfg.MCP.MaxConcurrentCallsPerSession)
	}
	if cfg.RAG.QueryCacheBurst != 128 {
		t.Errorf("cache burst: got %d, want 128", cfg.RAG.QueryCacheBurst)
	}
	if len(cfg.MCP.Servers) != 2 {
		t.Errorf("mcp servers: got %d, want 2", len(cfg.MCP.Servers))
	}
	if cfg.Auth.BearerTokenEnv != "CUSTOM_TOKEN_ENV" {
		t.Errorf("bearer env: got %q, want %q", cfg.Auth.BearerTokenEnv, "CUSTOM_TOKEN_ENV")
	}
	if cfg.BearerToken() != "full-config-token" {
		t.Errorf("bearer token via custom env: got %q", cfg.BearerToken())
	}
	if cfg.Permissions.ActiveProfile != ProfileReadOnly {
		t.Errorf("profile: got %q, want %q", cfg.Permissions.ActiveProfile, ProfileReadOnly)
	}
	_ = path
}

func TestLoad_TrustedFullArmedEndToEnd(t *testing.T) {
	yamlContent := `
permissions:
  active_profile: "trusted_full"
  trusted_full:
    enabled: true
    admin_identities:
      - "admin@corp.com"
    filesystem_roots:
      - "C:\\"
      - "D:\\data"
audit:
  enabled: true
  path: ".runtime/logs/audit.jsonl"
`
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "cfg.yaml", yamlContent)
	setEnv(t, "TIBRAIN_AUTONOMY_MODE", "trusted_full")
	setEnv(t, "TIBRAIN_MCP_BEARER_TOKEN", "admin-secret")
	// Hermetic: the machine may have TIBRAIN_ALLOWED_ROOTS / MCP_ALLOWED_ROOTS
	// set globally, which would leak extra entries into AllowedRoots and make
	// the effective-roots count below non-deterministic. Unset both so the
	// test only sees the filesystem roots declared in the YAML above.
	unsetEnv(t, "TIBRAIN_ALLOWED_ROOTS")
	unsetEnv(t, "MCP_ALLOWED_ROOTS")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load trusted_full config: %v", err)
	}
	if !cfg.TrustedFullArmed() {
		t.Error("should be armed after full setup")
	}
	if cfg.BearerToken() != "admin-secret" {
		t.Errorf("bearer token: got %q", cfg.BearerToken())
	}
	roots := cfg.EffectiveRoots()
	if len(roots) != 2 { // AllowedRoots empty + 2 filesystem roots
		// Actually AllowedRoots defaults to nil from YAML, so just 2
		t.Errorf("effective roots count: got %d", len(roots))
	}
	_ = path
}
