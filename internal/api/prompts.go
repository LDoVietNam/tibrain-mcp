package api

import (
	"encoding/json"
	"net/http"
)

// PromptPreset represents a single prompt preset for Tirouter compatibility
type PromptPreset struct {
	Name   string   `json:"name"`
	Models []string `json:"models,omitempty"`
	Match  []string `json:"match,omitempty"`
	Inject struct {
		System      string `json:"system,omitempty"`
		SystemMode  string `json:"system_mode,omitempty"`
		PrependUser string `json:"prepend_user,omitempty"`
		AppendUser  string `json:"append_user,omitempty"`
	} `json:"inject"`
}

// PromptConfig represents the prompt configuration format expected by Tirouter
type PromptConfig struct {
	Enabled bool `json:"enabled"`
	Default struct {
		System      string `json:"system,omitempty"`
		SystemMode  string `json:"system_mode,omitempty"`
		PrependUser string `json:"prepend_user,omitempty"`
		AppendUser  string `json:"append_user,omitempty"`
	} `json:"default"`
	Presets []PromptPreset `json:"presets"`
}

// PromptAPIHandler handles prompt-related API endpoints
type PromptAPIHandler struct{}

// NewPromptAPIHandler creates a new prompt API handler
func NewPromptAPIHandler() *PromptAPIHandler {
	return &PromptAPIHandler{}
}

// ServeHTTP handles requests to /api/prompts
func (h *PromptAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIError{
			Code:    "method_not_allowed",
			Message: "GET method required",
		})
		return
	}

	// Return default prompt configuration
	// This can be extended to fetch from database or config in the future
	cfg := PromptConfig{
		Enabled: false,
		Default: struct {
			System      string `json:"system,omitempty"`
			SystemMode  string `json:"system_mode,omitempty"`
			PrependUser string `json:"prepend_user,omitempty"`
			AppendUser  string `json:"append_user,omitempty"`
		}{
			SystemMode: "merge",
		},
		Presets: []PromptPreset{
			{
				Name:   "vietnamese-default",
				Models: []string{"*"},
				Inject: struct {
					System      string `json:"system,omitempty"`
					SystemMode  string `json:"system_mode,omitempty"`
					PrependUser string `json:"prepend_user,omitempty"`
					AppendUser  string `json:"append_user,omitempty"`
				}{
					AppendUser: "Luôn trả lời bằng tiếng Việt.",
				},
			},
			{
				Name:   "code-review",
				Models: []string{"gpt-*", "claude-*"},
				Match:  []string{"metadata.mode=review"},
				Inject: struct {
					System      string `json:"system,omitempty"`
					SystemMode  string `json:"system_mode,omitempty"`
					PrependUser string `json:"prepend_user,omitempty"`
					AppendUser  string `json:"append_user,omitempty"`
				}{
					System:     "Bạn là reviewer nghiêm ngặt. Luôn trả lời tiếng Việt.",
					SystemMode: "merge",
					AppendUser: "Trình bày theo bullet point.",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}
