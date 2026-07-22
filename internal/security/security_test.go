package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ti/router/tibrain/internal/config"
)

func TestAuthenticate_MissingToken(t *testing.T) {
	a := NewAuthenticator("secret", []string{"https://chatgpt.com"}, 60)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	_, err := a.Authenticate(req)
	if err == nil {
		t.Fatal("expected 401 for missing token")
	}
	if ae, ok := err.(*AuthError); !ok || ae.Status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestAuthenticate_WrongToken(t *testing.T) {
	a := NewAuthenticator("secret", []string{"https://chatgpt.com"}, 60)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	_, err := a.Authenticate(req)
	if err == nil || err.(*AuthError).Status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %v", err)
	}
}

func TestAuthenticate_OK(t *testing.T) {
	a := NewAuthenticator("secret", []string{"https://chatgpt.com"}, 60)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	id, err := a.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "anonymous" || id == "" {
		t.Fatalf("expected resolved identity, got %q", id)
	}
}

func TestAuthenticate_BadOrigin(t *testing.T) {
	a := NewAuthenticator("secret", []string{"https://chatgpt.com"}, 60)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "https://evil.example.com")
	_, err := a.Authenticate(req)
	if err == nil || err.(*AuthError).Status != http.StatusForbidden {
		t.Fatalf("expected 403 for bad origin, got %v", err)
	}
}

func TestAuthenticate_NoTokenConfiguredClosed(t *testing.T) {
	a := NewAuthenticator("", []string{"https://chatgpt.com"}, 60)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	_, err := a.Authenticate(req)
	if err == nil || err.(*AuthError).Status != http.StatusUnauthorized {
		t.Fatalf("expected 401 when gateway auth unconfigured, got %v", err)
	}
}

func TestGuard_OperatorDeniesDestructive(t *testing.T) {
	g := NewGuard(&config.Config{Permissions: config.PermissionsConfig{ActiveProfile: config.ProfileOperator}})
	if err := g.Allow("tok:abc", CatDestruct); err == nil {
		t.Fatal("operator must deny destructive")
	}
	if err := g.Allow("tok:abc", CatRead); err != nil {
		t.Fatalf("operator should allow read: %v", err)
	}
	if err := g.Allow("tok:abc", CatWrite); err != nil {
		t.Fatalf("operator should allow write: %v", err)
	}
}

func TestGuard_ReadOnlyDeniesWrite(t *testing.T) {
	g := NewGuard(&config.Config{Permissions: config.PermissionsConfig{ActiveProfile: config.ProfileReadOnly}})
	if err := g.Allow("tok:abc", CatWrite); err == nil {
		t.Fatal("read_only must deny write")
	}
	if err := g.Allow("tok:abc", CatRead); err != nil {
		t.Fatalf("read_only should allow read: %v", err)
	}
}

func TestRedact(t *testing.T) {
	if Redact("short") != "short" {
		t.Fatal("short string should not be redacted")
	}
	long := "abcdef0123456789abcdef0123456789"
	if Redact(long) == long {
		t.Fatal("long high-entropy string should be redacted")
	}
}

func TestRedactMap(t *testing.T) {
	m := map[string]string{"token": "abc123", "name": "ok"}
	out := RedactMap(m)
	if out["token"] != "***REDACTED***" {
		t.Fatalf("token should be redacted, got %q", out["token"])
	}
	if out["name"] != "ok" {
		t.Fatalf("non-sensitive should pass through, got %q", out["name"])
	}
}
