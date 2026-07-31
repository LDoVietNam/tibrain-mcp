package skills

import "context"

// Skill represents a skill that can be used by the AI system
// Based on requirements:
// skill_id
// name
// description
// intent_patterns
// capabilities
// risk_level
// success_rate
// usage_count
// version
type Skill struct {
	ID             string
	Name           string
	Description    string
	IntentPatterns []string
	Capabilities   []string
	RiskLevel      string
	SuccessRate    float64
	UsageCount     int
	Version        string
}

// SkillRegistry manages the collection of available skills
type SkillRegistry interface {
	// Register adds a skill to the registry
	Register(ctx context.Context, skill Skill) error

	// Get retrieves a skill by ID
	Get(ctx context.Context, id string) (*Skill, error)

	// List returns all registered skills
	List(ctx context.Context) ([]Skill, error)
}
