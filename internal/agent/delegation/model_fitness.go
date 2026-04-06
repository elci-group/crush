package delegation

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

// ModelFitness represents how well a model suits a particular task.
type ModelFitness struct {
	// Model identifier
	Model    config.SelectedModel `json:"model"`
	Provider string               `json:"provider"`

	// Fitness score [0,1]
	Score float64 `json:"score"`

	// Component scores
	ReasoningFitness    float64 `json:"reasoning_fitness"`     // Does model handle complex reasoning?
	LatencyFitness      float64 `json:"latency_fitness"`       // Is latency acceptable?
	CostFitness         float64 `json:"cost_fitness"`          // Cost-benefit ratio
	SpecializationFit   float64 `json:"specialization_fit"`    // How specialized for this task?
	HistoricalSuccess   float64 `json:"historical_success"`    // Prior success on similar tasks
	ContextWindowFit    float64 `json:"context_window_fit"`    // Is context window sufficient?

	// Reasoning for selection
	Reasoning string `json:"reasoning"`
}

// ModelSelector scores and ranks models for a task.
type ModelSelector struct {
	availableModels []config.SelectedModel
	calibrator      *Calibrator
}

// NewModelSelector creates a model selector with calibration.
func NewModelSelector(models []config.SelectedModel, calibrator *Calibrator) *ModelSelector {
	return &ModelSelector{
		availableModels: models,
		calibrator:      calibrator,
	}
}

// ScoreModelsForTask evaluates all available models against a task.
func (ms *ModelSelector) ScoreModelsForTask(task *SubTask) ([]ModelFitness, error) {
	var fitnesses []ModelFitness

	for _, model := range ms.availableModels {
		fitness := ms.scoreModelForTask(model, task)
		fitnesses = append(fitnesses, fitness)
	}

	// Sort by score (highest first)
	sortByFitness(fitnesses)

	return fitnesses, nil
}

// scoreModelForTask evaluates a single model for a task.
func (ms *ModelSelector) scoreModelForTask(model config.SelectedModel, task *SubTask) ModelFitness {
	fitness := ModelFitness{
		Model:    model,
		Provider: model.Provider,
	}

	// Score components
	fitness.ReasoningFitness = ms.scoreReasoningFitness(model, task)
	fitness.LatencyFitness = ms.scoreLatencyFitness(model, task)
	fitness.CostFitness = ms.scoreCostFitness(model, task)
	fitness.SpecializationFit = ms.scoreSpecialization(model, task)
	fitness.HistoricalSuccess = ms.scoreHistoricalSuccess(model, task)
	fitness.ContextWindowFit = ms.scoreContextWindow(model, task)

	// Weighted overall score
	// Weights: reasoning=0.25, specialization=0.25, history=0.2, latency=0.15, cost=0.1, window=0.05
	fitness.Score = (0.25 * fitness.ReasoningFitness) +
		(0.25 * fitness.SpecializationFit) +
		(0.2 * fitness.HistoricalSuccess) +
		(0.15 * fitness.LatencyFitness) +
		(0.1 * fitness.CostFitness) +
		(0.05 * fitness.ContextWindowFit)

	fitness.Reasoning = ms.generateReasoningForFitness(fitness)

	return fitness
}

// scoreReasoningFitness evaluates if model can handle task complexity [0,1].
func (ms *ModelSelector) scoreReasoningFitness(model config.SelectedModel, task *SubTask) float64 {
	// Task complexity proxy: estimated complexity value
	complexity := float64(task.EstimatedComplexity) / 10.0 // normalize to [0,1]

	// Models with reasoning capabilities
	reasoningCapable := map[string]bool{
		"claude-3.5": true,
		"gpt-4":      true,
		"o1":         true,
		"reasoning":  true,
	}

	score := 0.5 // baseline
	lower := strings.ToLower(model.Model)

	// High complexity tasks need reasoning models
	if complexity > 0.7 {
		for model := range reasoningCapable {
			if strings.Contains(lower, model) {
				score = 0.9
				break
			}
		}
	} else {
		// Low complexity tasks don't need premium reasoning
		score = 0.7
	}

	return score
}

// scoreLatencyFitness evaluates latency tolerance [0,1].
func (ms *ModelSelector) scoreLatencyFitness(model config.SelectedModel, task *SubTask) float64 {
	// Fast-response models (e.g., Haiku, smaller models)
	fastModels := map[string]float64{
		"haiku":    0.95,
		"gpt-3.5": 0.9,
		"mini":     0.9,
	}

	lower := strings.ToLower(model.Model)
	for fastModel, score := range fastModels {
		if strings.Contains(lower, fastModel) {
			return score
		}
	}

	// Default: medium latency
	return 0.7
}

// scoreCostFitness evaluates cost-benefit [0,1].
func (ms *ModelSelector) scoreCostFitness(model config.SelectedModel, task *SubTask) float64 {
	// Estimate token count for task
	estimatedTokens := ms.estimateTaskTokens(task)

	// Cheaper models for smaller tasks
	budget := float64(estimatedTokens) / 10000.0 // cost proxy
	if budget < 0.5 {
		// Very cheap task - any model is fine
		return 0.8
	}

	// For expensive tasks, prefer cheaper models
	cheaperModels := map[string]bool{
		"gpt-3.5": true,
		"haiku":   true,
		"claude-instant": true,
	}

	lower := strings.ToLower(model.Model)
	for cheapModel := range cheaperModels {
		if strings.Contains(lower, cheapModel) {
			return 0.85
		}
	}

	// Premium models for premium tasks (okay cost)
	return 0.7
}

// scoreSpecialization evaluates if model is specialized for task domain [0,1].
func (ms *ModelSelector) scoreSpecialization(model config.SelectedModel, task *SubTask) float64 {
	lower := strings.ToLower(task.Title)

	// Domain-specific model matching
	specializations := map[string][]string{
		"auth":       {"claude", "gpt-4"},
		"api":        {"gpt-4", "claude"},
		"database":   {"gpt-4", "claude-opus"},
		"ui":         {"gpt-4", "claude"},
		"cache":      {"gpt-3.5", "claude"},
		"logging":    {"gpt-3.5", "haiku"},
		"middleware": {"gpt-4", "claude"},
	}

	score := 0.6 // baseline
	modelLower := strings.ToLower(model.Model)

	for domain, specialized := range specializations {
		if strings.Contains(lower, domain) {
			for _, modelPattern := range specialized {
				if strings.Contains(modelLower, modelPattern) {
					score = 0.85
					break
				}
			}
		}
	}

	return score
}

// scoreHistoricalSuccess evaluates prior success on similar tasks [0,1].
func (ms *ModelSelector) scoreHistoricalSuccess(model config.SelectedModel, task *SubTask) float64 {
	if ms.calibrator == nil {
		return 0.6 // neutral if no history
	}

	history := ms.calibrator.GetExecutionHistory()
	if len(history) == 0 {
		return 0.6 // no history yet
	}

	// Count successes with this model
	successes := 0
	total := 0

	for _, execution := range history {
		// Match by model name (simplified - would use actual model tracking)
		if strings.Contains(execution.OriginalScore.DecompositionStrategy.Rationale, model.Model) {
			total++
			if execution.DelegationEffectiveness > 0.7 {
				successes++
			}
		}
	}

	if total == 0 {
		return 0.6 // new model
	}

	return float64(successes) / float64(total)
}

// scoreContextWindow evaluates if model's context window is sufficient [0,1].
func (ms *ModelSelector) scoreContextWindow(model config.SelectedModel, task *SubTask) float64 {
	estimatedTokens := ms.estimateTaskTokens(task)

	// Model context window estimates (simplified)
	contextWindows := map[string]int{
		"gpt-4":            8192,
		"gpt-3.5":          4096,
		"haiku":            200000,
		"claude-3.5":       200000,
		"claude-opus":      200000,
		"claude-sonnet":    200000,
	}

	defaultWindow := 8000 // conservative default

	lower := strings.ToLower(model.Model)
	contextWindow := defaultWindow
	for pattern, size := range contextWindows {
		if strings.Contains(lower, pattern) {
			contextWindow = size
			break
		}
	}

	// Buffer: need 2x estimated tokens to be comfortable
	requiredWindow := estimatedTokens * 2

	if requiredWindow < contextWindow {
		// Has sufficient buffer
		ratio := float64(contextWindow) / float64(requiredWindow)
		return math.Min(1.0, ratio/3.0) // 3x+ buffer = perfect score
	}

	// Insufficient window
	return float64(contextWindow) / float64(requiredWindow)
}

// estimateTaskTokens estimates token count for a task.
func (ms *ModelSelector) estimateTaskTokens(task *SubTask) int {
	// Very rough estimate: ~1 token per 4 chars
	titleTokens := len(task.Title) / 4
	descTokens := len(task.Description) / 4
	scopeTokens := len(task.Scope.Description) / 4

	// Add base overhead
	return titleTokens + descTokens + scopeTokens + 200
}

// generateReasoningForFitness creates human-readable explanation.
func (ms *ModelSelector) generateReasoningForFitness(fitness ModelFitness) string {
	reasons := []string{}

	if fitness.SpecializationFit > 0.8 {
		reasons = append(reasons, "specialized for domain")
	}
	if fitness.ReasoningFitness > 0.8 {
		reasons = append(reasons, "strong reasoning")
	}
	if fitness.LatencyFitness > 0.8 {
		reasons = append(reasons, "low latency")
	}
	if fitness.HistoricalSuccess > 0.8 {
		reasons = append(reasons, "proven success rate")
	}
	if fitness.CostFitness < 0.5 {
		reasons = append(reasons, "cost-effective")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "balanced profile")
	}

	return fmt.Sprintf("Selected for: %s", strings.Join(reasons, ", "))
}

// sortByFitness sorts fitnesses by score descending.
func sortByFitness(fitnesses []ModelFitness) {
	for i := 0; i < len(fitnesses); i++ {
		for j := i + 1; j < len(fitnesses); j++ {
			if fitnesses[j].Score > fitnesses[i].Score {
				fitnesses[i], fitnesses[j] = fitnesses[j], fitnesses[i]
			}
		}
	}
}
