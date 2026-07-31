package prompt

import "time"

type PromptCapsule struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Intent        string        `json:"intent"`
	Domain        string        `json:"domain,omitempty"`
	Risk          CapsuleRisk   `json:"risk"`
	Status        CapsuleStatus `json:"status"`
	SourceURL     string        `json:"source_url,omitempty"`
	SourceType    string        `json:"source_type,omitempty"`
	RetrievedAt   int64         `json:"retrieved_at,omitempty"`
	LicenseStatus string        `json:"license_status,omitempty"`
	CreatedAt     int64         `json:"created_at"`
	UpdatedAt     int64         `json:"updated_at"`
}

// PromptVersion represents a version of a prompt capsule.
// ID is the capsule ID (foreign key to prompt_capsules.id); the composite
// primary key in the DB is (capsule_id, version).
type PromptVersion struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Content       string `json:"content"`
	ContentHash   string `json:"content_hash"`
	TokenEstimate int    `json:"token_estimate,omitempty"`
	Placement     string `json:"placement"`
	CreatedAt     int64  `json:"created_at"`
}

type PromptTrace struct {
	ID             string   `json:"id"`
	RequestID      string   `json:"request_id"`
	CapsuleID      string   `json:"capsule_id"`
	CapsuleVersion string   `json:"capsule_version,omitempty"`
	Decision       Decision `json:"decision"`
	Confidence     float64  `json:"confidence,omitempty"`
	ReasonCode     string   `json:"reason_code,omitempty"`
	LatencyMs      int      `json:"latency_ms,omitempty"`
	CreatedAt      int64    `json:"created_at"`
}

type PromptFeedback struct {
	ID                string `json:"id"`
	RequestID         string `json:"request_id"`
	CapsuleID         string `json:"capsule_id"`
	CapsuleVersion    string `json:"capsule_version"`
	Outcome           string `json:"outcome"`
	UserOverride      bool   `json:"user_override"`
	AddedTokens       int    `json:"added_tokens,omitempty"`
	ProviderErrorCode string `json:"provider_error_code,omitempty"`
	CreatedAt         int64  `json:"created_at"`
}

type PromptEvaluation struct {
	ID              string   `json:"id"`
	DatasetCase     string   `json:"dataset_case"`
	ExpectedCapsule string   `json:"expected_capsule,omitempty"`
	ActualCapsule   string   `json:"actual_capsule,omitempty"`
	Decision        Decision `json:"decision,omitempty"`
	Confidence      float64  `json:"confidence,omitempty"`
	CreatedAt       int64    `json:"created_at"`
}

type PreflightRequest struct {
	Intent      string         `json:"intent"`
	Domain      string         `json:"domain,omitempty"`
	Models      []string       `json:"models,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	MaxCapsules int            `json:"max_capsules,omitempty"`
}

type FeedbackRequest struct {
	RequestID         string `json:"request_id"`
	CapsuleID         string `json:"capsule_id"`
	CapsuleVersion    string `json:"capsule_version"`
	Outcome           string `json:"outcome"`
	UserOverride      bool   `json:"user_override"`
	AddedTokens       int    `json:"added_tokens,omitempty"`
	ProviderErrorCode string `json:"provider_error_code,omitempty"`
}

type ObserveRequest struct {
	RequestID      string            `json:"request_id"`
	CapsuleID      string            `json:"capsule_id"`
	CapsuleVersion string            `json:"capsule_version"`
	Decision       string            `json:"decision"`
	Confidence     float64           `json:"confidence"`
	ReasonCode     string            `json:"reason_code"`
	LatencyMs      int               `json:"latency_ms"`
	Outcome        string            `json:"outcome,omitempty"`
	UserOverride   bool              `json:"user_override,omitempty"`
	AddedTokens    int               `json:"added_tokens,omitempty"`
	Providers      []string          `json:"providers,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ObserveResponse struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
	CapsuleID string `json:"capsule_id,omitempty"`
}

type MetricsResponse struct {
	TotalCapsules     int64          `json:"total_capsules"`
	TotalVersions     int64          `json:"total_versions"`
	TotalTraces       int64          `json:"total_traces"`
	TotalFeedback     int64          `json:"total_feedback"`
	TotalEvaluations  int64          `json:"total_evaluations"`
	AvgConfidence     float64        `json:"avg_confidence"`
	DecisionBreakdown map[string]int `json:"decision_breakdown,omitempty"`
}

type CanaryRequest struct {
	Version   string   `json:"version"`
	Weight    int      `json:"weight,omitempty"`
	Duration  string   `json:"duration,omitempty"`
	Allowlist []string `json:"allowlist,omitempty"`
}

type Metrics struct {
	TotalCapsules    int64   `json:"total_capsules"`
	TotalVersions    int64   `json:"total_versions"`
	TotalTraces      int64   `json:"total_traces"`
	TotalFeedback    int64   `json:"total_feedback"`
	TotalEvaluations int64   `json:"total_evaluations"`
	AvgConfidence    float64 `json:"avg_confidence,omitempty"`
}

var timeNow = func() int64 {
	return time.Now().Unix()
}
