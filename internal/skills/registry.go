package skills

import (
	"context"
	"sync"
)

// InMemorySkillRegistry is a simple in-memory implementation of SkillRegistry
type InMemorySkillRegistry struct {
	skills map[string]*Skill
	mu     sync.RWMutex
}

// NewInMemorySkillRegistry creates a new in-memory skill registry
func NewInMemorySkillRegistry() *InMemorySkillRegistry {
	return &InMemorySkillRegistry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a skill to the registry
func (isr *InMemorySkillRegistry) Register(ctx context.Context, skill Skill) error {
	// Basic validation
	if skill.ID == "" {
		return ErrInvalidSkillID
	}
	if skill.Name == "" {
		return ErrInvalidSkillName
	}

	isr.mu.Lock()
	defer isr.mu.Unlock()

	// Store a copy to prevent external modification
	isr.skills[skill.ID] = &Skill{
		ID:             skill.ID,
		Name:           skill.Name,
		Description:    skill.Description,
		IntentPatterns: append([]string{}, skill.IntentPatterns...),
		Capabilities:   append([]string{}, skill.Capabilities...),
		RiskLevel:      skill.RiskLevel,
		SuccessRate:    skill.SuccessRate,
		UsageCount:     skill.UsageCount,
		Version:        skill.Version,
	}
	return nil
}

// Get retrieves a skill by ID
func (isr *InMemorySkillRegistry) Get(ctx context.Context, id string) (*Skill, error) {
	isr.mu.RLock()
	defer isr.mu.RUnlock()

	if s, exists := isr.skills[id]; exists {
		// Return a copy to prevent external modification
		return &Skill{
			ID:             s.ID,
			Name:           s.Name,
			Description:    s.Description,
			IntentPatterns: append([]string{}, s.IntentPatterns...),
			Capabilities:   append([]string{}, s.Capabilities...),
			RiskLevel:      s.RiskLevel,
			SuccessRate:    s.SuccessRate,
			UsageCount:     s.UsageCount,
			Version:        s.Version,
		}, nil
	}
	return nil, ErrSkillNotFound
}

// List returns all registered skills
func (isr *InMemorySkillRegistry) List(ctx context.Context) ([]Skill, error) {
	isr.mu.RLock()
	defer isr.mu.RUnlock()

	result := make([]Skill, 0, len(isr.skills))
	for _, s := range isr.skills {
		result = append(result, Skill{
			ID:             s.ID,
			Name:           s.Name,
			Description:    s.Description,
			IntentPatterns: append([]string{}, s.IntentPatterns...),
			Capabilities:   append([]string{}, s.Capabilities...),
			RiskLevel:      s.RiskLevel,
			SuccessRate:    s.SuccessRate,
			UsageCount:     s.UsageCount,
			Version:        s.Version,
		})
	}
	return result, nil
}

// Error definitions
var (
	ErrInvalidSkillID   = Error("invalid skill ID")
	ErrInvalidSkillName = Error("invalid skill name")
	ErrSkillNotFound    = Error("skill not found")
)

// Error is a simple error implementation
type Error string

func (e Error) Error() string { return string(e) }
