// Package presets provides preset tool sets that surface named, curated
// selections of MCP tools for different use cases (minimal, standard, coding).
// Presets act as filters applied on top of the global registry.
package presets

import "strings"

// PresetName identifies a known preset.
type PresetName string

const (
	PresetMinimal  PresetName = "minimal"
	PresetStandard PresetName = "standard"
	PresetCoding   PresetName = "coding"
	PresetFull     PresetName = "full"
	PresetCustom   PresetName = "custom"
)

// Preset defines a curated set of MCP tool categories and tool name patterns.
type Preset struct {
	Name        PresetName // Preset identifier
	Description string     // Human-readable description
	Categories  []string   // Allowed security categories (e.g. "read", "write", "destruct")
	Patterns    []string   // Glob patterns for tool names (e.g. "fs.*", "shell.*")
	Exclude     []string   // Tool name patterns to explicitly exclude
	Enabled     bool       // Whether this preset is active
}

// Registry holds the set of available presets.
type Registry struct {
	presets map[PresetName]*Preset
}

// NewRegistry creates a preset registry with built-in presets.
func NewRegistry() *Registry {
	r := &Registry{
		presets: make(map[PresetName]*Preset),
	}
	r.registerDefaults()
	return r
}

func (r *Registry) registerDefaults() {
	r.Register(&Preset{
		Name:        PresetMinimal,
		Description: "Minimal preset: only read-only filesystem tools",
		Categories:  []string{"read"},
		Patterns:    []string{"fs.*", "brain.*"},
		Enabled:     true,
	})
	r.Register(&Preset{
		Name:        PresetStandard,
		Description: "Standard preset: read + write tools (fs, shell, process, git, android, pocketmcp)",
		Categories:  []string{"read", "write"},
		Patterns:    []string{"fs.*", "shell.*", "process.*", "git.*", "brain.*", "android.*", "pocketmcp.*"},
		Enabled:     true,
	})
	r.Register(&Preset{
		Name:        PresetCoding,
		Description: "Coding preset: standard + browser, http, database tools",
		Categories:  []string{"read", "write", "destruct"},
		Patterns:    []string{"fs.*", "shell.*", "process.*", "git.*", "browser.*", "http.*", "db.*", "brain.*"},
		Exclude:     []string{"shell.rm", "shell.format.*"},
		Enabled:     true,
	})
	r.Register(&Preset{
		Name:        PresetFull,
		Description: "Full preset: all tools from all categories",
		Categories:  []string{"read", "write", "destruct"},
		Patterns:    []string{"*"},
		Enabled:     true,
	})
}

// Register adds or updates a preset in the registry.
func (r *Registry) Register(p *Preset) {
	r.presets[PresetName(p.Name)] = p
}

// Get retrieves a preset by name. Returns nil if not found.
func (r *Registry) Get(name PresetName) *Preset {
	return r.presets[name]
}

// List returns all registered presets.
func (r *Registry) List() []*Preset {
	result := make([]*Preset, 0, len(r.presets))
	for _, p := range r.presets {
		result = append(result, p)
	}
	return result
}

// ToolAllowed checks whether a tool is permitted by a preset.
// It checks category matches and pattern inclusion/exclusion.
func (r *Registry) ToolAllowed(presetName PresetName, toolName string, category string) bool {
	p := r.presets[presetName]
	if p == nil || !p.Enabled {
		return false
	}

	// Check explicit exclusion first
	for _, excl := range p.Exclude {
		if matchPattern(excl, toolName) {
			return false
		}
	}

	// Check category
	catAllowed := false
	for _, c := range p.Categories {
		if c == category {
			catAllowed = true
			break
		}
	}
	if !catAllowed {
		return false
	}

	// Check pattern inclusion
	for _, pat := range p.Patterns {
		if matchPattern(pat, toolName) {
			return true
		}
	}

	return false
}

// matchPattern checks if a tool name matches a glob-like pattern.
// Supports "*" as wildcard (matches any substring).
func matchPattern(pattern, toolName string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == toolName {
		return true
	}
	// Simple wildcard: prefix* or *suffix or *infix*
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(toolName, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(toolName, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(toolName, pattern[:len(pattern)-1])
	}
	return false
}
