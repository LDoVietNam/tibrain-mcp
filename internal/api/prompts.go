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
			{
				Name:   "intelligence-first",
				Models: []string{"*"},
				Match:  []string{"metadata.mode=planning", "metadata.mode=implementation", "metadata.intent=prioritize"},
				Inject: struct {
					System      string `json:"system,omitempty"`
					SystemMode  string `json:"system_mode,omitempty"`
					PrependUser string `json:"prepend_user,omitempty"`
					AppendUser  string `json:"append_user,omitempty"`
				}{
					System:     "Nguyên tắc ưu tiên thông minh: Tuyệt đối không lãng phí năng lượng cho những câu hỏi vô dụng như 'implement cái nào trước?', 'bắt đầu với cái nào trước?'. Luôn tự động xác định ưu tiên dựa trên dependency graph, impact analysis, và risk assessment. Sử dụng model AI để đưa ra quyết định thay vì hỏi người dùng. Nếu có nhiều task, tự động sắp xếp theo priority và execute.",
					SystemMode: "merge",
					AppendUser: "Tự động xác định ưu tiên và thực hiện. Không hỏi 'bắt đầu với cái nào'.",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}
