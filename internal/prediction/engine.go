package prediction

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PredictionEngine predicts the best agent, skill, and tool for a task
type PredictionEngine struct {
	models    []Model
	tools     []Tool
	skills    []Skill
	knowledge KnowledgeProvider
}

// KnowledgeProvider provides access to knowledge store
type KnowledgeProvider interface {
	Query(ctx context.Context, query string) ([]interface{}, error)
}

// Model represents a language model
type Model struct {
	ID           string
	Name         string
	Capabilities []string
	QualityScore float64
	Latency      float64
	Cost         float64
}

// Tool represents an executable tool
type Tool struct {
	ID            string
	Name          string
	Category      string
	Description   string
	QualityScore  float64
	SecurityScore float64
}

// Skill represents a skill/prompt
type Skill struct {
	ID             string
	Name           string
	Domain         string
	IntentPatterns []string
	QualityScore   float64
}

// NewPredictionEngine creates a new prediction engine
func NewPredictionEngine() *PredictionEngine {
	return &PredictionEngine{
		models: make([]Model, 0),
		tools:  make([]Tool, 0),
		skills: make([]Skill, 0),
	}
}

// AddModel adds a model to the prediction engine
func (e *PredictionEngine) AddModel(model Model) {
	e.models = append(e.models, model)
}

// AddTool adds a tool to the prediction engine
func (e *PredictionEngine) AddTool(tool Tool) {
	e.tools = append(e.tools, tool)
}

// AddSkill adds a skill to the prediction engine
func (e *PredictionEngine) AddSkill(skill Skill) {
	e.skills = append(e.skills, skill)
}

// Predict predicts the best model, skill, and tool for a task
func (e *PredictionEngine) Predict(ctx context.Context, task string) (*PredictionResult, error) {
	// Extract intent
	intent := e.extractIntent(task)

	// Predict skill
	skill := e.predictSkill(intent)

	// Predict tool
	tool := e.predictTool(skill)

	// Predict model
	model := e.predictModel(skill)

	return &PredictionResult{
		Task:      task,
		Intent:    intent,
		Skill:     skill,
		Tool:      tool,
		Model:     model,
		Score:     e.calculateScore(skill, tool, model),
		CreatedAt: time.Now(),
	}, nil
}

// PredictionResult represents the prediction result
type PredictionResult struct {
	Task      string
	Intent    string
	Skill     *Skill
	Tool      *Tool
	Model     *Model
	Score     float64
	CreatedAt time.Time
}

func (e *PredictionEngine) extractIntent(task string) string {
	task = strings.ToLower(task)

	// Simple intent extraction based on keywords
	if strings.Contains(task, "list") || strings.Contains(task, "search") || strings.Contains(task, "find") {
		return "list"
	}
	if strings.Contains(task, "create") || strings.Contains(task, "write") || strings.Contains(task, "generate") {
		return "create"
	}
	if strings.Contains(task, "read") || strings.Contains(task, "get") || strings.Contains(task, "fetch") {
		return "read"
	}
	if strings.Contains(task, "analyze") || strings.Contains(task, "review") || strings.Contains(task, "audit") {
		return "analyze"
	}
	if strings.Contains(task, "delete") || strings.Contains(task, "remove") || strings.Contains(task, "clean") {
		return "delete"
	}

	return "execute"
}

func (e *PredictionEngine) predictSkill(intent string) *Skill {
	// Find skill that matches intent
	for _, skill := range e.skills {
		for _, pattern := range skill.IntentPatterns {
			if strings.Contains(strings.ToLower(pattern), strings.ToLower(intent)) {
				return &skill
			}
		}
	}

	// Return default skill
	if len(e.skills) > 0 {
		return &e.skills[0]
	}

	return &Skill{
		ID:             "default",
		Name:           "Default Skill",
		Domain:         "general",
		IntentPatterns: []string{intent},
		QualityScore:   0.5,
	}
}

func (e *PredictionEngine) predictTool(skill *Skill) *Tool {
	// Find tool in same domain
	for _, tool := range e.tools {
		if strings.Contains(strings.ToLower(tool.Category), strings.ToLower(skill.Domain)) {
			return &tool
		}
	}

	// Return default tool
	if len(e.tools) > 0 {
		return &e.tools[0]
	}

	return &Tool{
		ID:            "default",
		Name:          "default_tool",
		Category:      "general",
		Description:   "Default tool",
		QualityScore:  0.5,
		SecurityScore: 0.5,
	}
}

func (e *PredictionEngine) predictModel(skill *Skill) *Model {
	// Find model with best quality for task
	var bestModel *Model
	bestScore := 0.0

	for i := range e.models {
		score := e.models[i].QualityScore
		if score > bestScore {
			bestScore = score
			bestModel = &e.models[i]
		}
	}

	if bestModel == nil {
		return &Model{
			ID:           "default",
			Name:         "Default Model",
			QualityScore: 0.5,
			Latency:      1.0,
			Cost:         0.01,
		}
	}

	return bestModel
}

func (e *PredictionEngine) calculateScore(skill *Skill, tool *Tool, model *Model) float64 {
	// Weighted score calculation
	skillWeight := 0.3
	toolWeight := 0.3
	modelWeight := 0.4

	score := skill.QualityScore*skillWeight + tool.QualityScore*toolWeight + model.QualityScore*modelWeight
	return score
}

// GetBestModel returns the best model for a given task type
func (e *PredictionEngine) GetBestModel(taskType string) (*Model, error) {
	var bestModel *Model
	bestScore := 0.0

	for i := range e.models {
		score := e.models[i].QualityScore
		if score > bestScore {
			bestScore = score
			bestModel = &e.models[i]
		}
	}

	if bestModel == nil {
		return nil, fmt.Errorf("no models available")
	}

	return bestModel, nil
}

// GetBestTool returns the best tool for a given category
func (e *PredictionEngine) GetBestTool(category string) (*Tool, error) {
	var bestTool *Tool
	bestScore := 0.0

	for i := range e.tools {
		score := e.tools[i].QualityScore
		if strings.Contains(strings.ToLower(e.tools[i].Category), strings.ToLower(category)) && score > bestScore {
			bestScore = score
			bestTool = &e.tools[i]
		}
	}

	if bestTool == nil {
		return nil, fmt.Errorf("no tools available for category: %s", category)
	}

	return bestTool, nil
}
