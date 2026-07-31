// Package templates provides server template resolution per client/session.
// Templates allow TiBrain to select the appropriate MCP server configuration
// based on the requesting client (e.g., Claude Desktop, Cursor, Termux agent).
package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TemplateName identifies a known client/session template.
type TemplateName string

const (
	TemplateClaudeDesktop TemplateName = "claude-desktop"
	TemplateCursor        TemplateName = "cursor"
	TemplateTermuxAgent   TemplateName = "termux-agent"
	TemplateCustom        TemplateName = "custom"
)

// ServerTemplate defines how an MCP server should be resolved for a given client.
type ServerTemplate struct {
	Name         string            // Template name
	Description  string            // Human-readable description
	ServerPrefix string            // Prefix for server names (e.g., "claude-")
	Env          map[string]string // Default env vars for this template
	Enabled      bool              // Whether this template is active
}

// Registry holds the set of available server templates.
type Registry struct {
	templates map[TemplateName]*ServerTemplate
}

// NewRegistry creates a new template registry with built-in templates.
func NewRegistry() *Registry {
	r := &Registry{
		templates: make(map[TemplateName]*ServerTemplate),
	}
	r.registerDefaults()
	return r
}

// Default templates shipped with TiBrain.
func (r *Registry) registerDefaults() {
	r.Register(&ServerTemplate{
		Name:         string(TemplateClaudeDesktop),
		Description:  "Template for Claude Desktop MCP clients",
		ServerPrefix: "claude-",
		Enabled:      true,
	})
	r.Register(&ServerTemplate{
		Name:         string(TemplateCursor),
		Description:  "Template for Cursor MCP clients",
		ServerPrefix: "cursor-",
		Enabled:      true,
	})
	r.Register(&ServerTemplate{
		Name:         string(TemplateTermuxAgent),
		Description:  "Template for Termux Android agent",
		ServerPrefix: "termux-",
		Env: map[string]string{
			"MCP_TERMINAL": "true",
		},
		Enabled: true,
	})
}

// Register adds or updates a template in the registry.
func (r *Registry) Register(t *ServerTemplate) {
	r.templates[TemplateName(t.Name)] = t
}

// Get retrieves a template by name. Returns nil if not found.
func (r *Registry) Get(name TemplateName) *ServerTemplate {
	return r.templates[name]
}

// List returns all registered templates.
func (r *Registry) List() []*ServerTemplate {
	result := make([]*ServerTemplate, 0, len(r.templates))
	for _, t := range r.templates {
		result = append(result, t)
	}
	return result
}

// ResolveServer returns the effective server name for a given template.
// If the template has a ServerPrefix, it prepends it to the bare server name.
func (r *Registry) ResolveServer(template TemplateName, bareName string) string {
	t := r.templates[template]
	if t == nil || !t.Enabled {
		return bareName
	}
	if t.ServerPrefix != "" {
		return t.ServerPrefix + bareName
	}
	return bareName
}

// ResolvePath resolves a template-relative path (e.g., prompts directory)
// based on the environment. For Termux, it uses $HOME/.config/tibrain.
func (r *Registry) ResolvePath(template TemplateName, relativePath string) (string, error) {
	t := r.templates[template]
	if t == nil || !t.Enabled {
		return "", fmt.Errorf("template %s not found or not enabled", template)
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", fmt.Errorf("cannot determine home directory")
	}

	// Termux-specific resolution
	if template == TemplateTermuxAgent {
		return filepath.Join(home, ".config", "tibrain", relativePath), nil
	}

	// Default resolution
	return filepath.Join(home, ".config", "tibrain", relativePath), nil
}

// TemplateForClient returns the best-matching template for a given client identifier.
// The clientID is typically sourced from request headers or CLI context.
func (r *Registry) TemplateForClient(clientID string) TemplateName {
	switch {
	case strings.Contains(strings.ToLower(clientID), "cursor"):
		return TemplateCursor
	case strings.Contains(strings.ToLower(clientID), "termux"),
		strings.Contains(strings.ToLower(clientID), "android"):
		return TemplateTermuxAgent
	case strings.Contains(strings.ToLower(clientID), "claude"),
		strings.Contains(strings.ToLower(clientID), "desktop"):
		return TemplateClaudeDesktop
	default:
		return TemplateCustom
	}
}

// EnsurePromptDir creates the prompts directory for a template if it does not exist.
func (r *Registry) EnsurePromptDir(template TemplateName) error {
	path, err := r.ResolvePath(template, "prompts")
	if err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}
