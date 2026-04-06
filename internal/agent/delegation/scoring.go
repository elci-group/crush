package delegation

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

// DelegabilityScore represents the delegatability assessment of a task.
type DelegabilityScore struct {
	// Overall delegatability score [0,1]
	Score float64 `json:"delegatability_score"`

	// Individual factor scores [0,1]
	Modularity         float64 `json:"modularity"`         // Can task be split into isolated scopes?
	Independence       float64 `json:"independence"`       // Can sub-tasks execute without blocking?
	SurfaceArea        float64 `json:"surface_area"`       // Number of affected modules
	CouplingRisk       float64 `json:"coupling_risk"`      // Merge conflict risk (negative weight)
	CognitiveLoad      float64 `json:"cognitive_load"`     // Reasoning depth required
	ParallelEfficiency float64 `json:"parallel_efficiency"` // Expected speedup from concurrency

	// Recommendation based on score
	Recommendation DelegationRecommendation `json:"recommendation"`

	// Confidence in the score [0,1]
	Confidence float64 `json:"confidence"`

	// Reasoning summary for the score
	ReasoningSummary string `json:"reasoning_summary"`

	// Decomposition strategy if delegating
	DecompositionStrategy *DecompositionStrategy `json:"decomposition_strategy,omitempty"`

	// Anti-delegation patterns detected
	AntiPatterns []string `json:"anti_patterns,omitempty"`
}

// DelegationRecommendation represents what action to take.
type DelegationRecommendation string

const (
	RecommendDelegate      DelegationRecommendation = "DELEGATE"
	RecommendSingleAgent   DelegationRecommendation = "SINGLE_AGENT_EXECUTION"
	RecommendStagedExec    DelegationRecommendation = "STAGED_EXECUTION"
	RecommendWithCaution   DelegationRecommendation = "DELEGATE_WITH_CAUTION"
)

// DecompositionStrategy suggests how to decompose the task.
type DecompositionStrategy struct {
	Method            string `json:"method"` // "module-based", "layer-based", "feature-sliced"
	EstimatedSubtasks int    `json:"estimated_subtasks"`
	Rationale         string `json:"rationale"`
}

// Scorer evaluates task delegatability.
type Scorer struct {
	cfg *config.Config
}

// NewScorer creates a new delegatability scorer.
func NewScorer(cfg *config.Config) *Scorer {
	return &Scorer{cfg: cfg}
}

// ScoreTask evaluates whether a task is suitable for delegation.
// Weights are: M=0.25, I=0.2, S=0.15, P=0.2, L=0.2, C=-0.3 (negative)
func (s *Scorer) ScoreTask(ctx context.Context, taskDescription string) (*DelegabilityScore, error) {
	score := &DelegabilityScore{
		AntiPatterns: []string{},
	}

	// Analyze task characteristics
	modularity := s.assessModularity(taskDescription)
	independence := s.assessIndependence(taskDescription)
	surfaceArea := s.assessSurfaceArea(taskDescription)
	couplingRisk := s.assessCouplingRisk(taskDescription)
	cognitiveLoad := s.assessCognitiveLoad(taskDescription)
	parallelEfficiency := s.assessParallelEfficiency(taskDescription)

	// Store individual scores
	score.Modularity = modularity
	score.Independence = independence
	score.SurfaceArea = surfaceArea
	score.CouplingRisk = couplingRisk
	score.CognitiveLoad = cognitiveLoad
	score.ParallelEfficiency = parallelEfficiency

	// Detect anti-delegation patterns
	antiPatterns := s.detectAntiPatterns(taskDescription, couplingRisk)
	score.AntiPatterns = antiPatterns

	// Compute weighted score: D = 0.25M + 0.2I + 0.15S + 0.2P + 0.2L - 0.3C
	delegabilityScore := (0.25 * modularity) +
		(0.2 * independence) +
		(0.15 * surfaceArea) +
		(0.2 * parallelEfficiency) +
		(0.2 * cognitiveLoad) -
		(0.3 * couplingRisk)

	// Clamp to [0, 1]
	score.Score = math.Max(0, math.Min(1, delegabilityScore))

	// Determine recommendation based on score and anti-patterns
	score.Recommendation = s.determineRecommendation(score.Score, antiPatterns, couplingRisk)

	// Set confidence based on data quality
	score.Confidence = s.computeConfidence(taskDescription, antiPatterns)

	// Generate reasoning summary
	score.ReasoningSummary = s.generateReasoning(score)

	// If delegating, suggest decomposition strategy
	if score.Recommendation == RecommendDelegate || score.Recommendation == RecommendWithCaution {
		score.DecompositionStrategy = s.suggestDecompositionStrategy(taskDescription, score)
	}

	return score, nil
}

// assessModularity evaluates if task can be split into isolated file scopes [0,1].
func (s *Scorer) assessModularity(task string) float64 {
	lower := strings.ToLower(task)

	// Positive indicators
	indicators := map[string]float64{
		"module":      0.15,
		"component":   0.15,
		"layer":       0.1,
		"service":     0.15,
		"api":         0.1,
		"ui":          0.1,
		"feature":     0.15,
		"auth":        0.15,
		"payment":     0.15,
		"cache":       0.1,
		"database":    0.1,
		"storage":     0.1,
		"logging":     0.1,
		"middleware":  0.1,
	}

	score := 0.3 // baseline
	for keyword, weight := range indicators {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	// Negative indicators
	negatives := map[string]float64{
		"monolithic": -0.2,
		"tightly":    -0.15,
		"global":     -0.1,
	}
	for keyword, weight := range negatives {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	return math.Max(0, math.Min(1, score))
}

// assessIndependence evaluates if sub-tasks can execute without blocking [0,1].
func (s *Scorer) assessIndependence(task string) float64 {
	lower := strings.ToLower(task)

	// Independence indicators
	independent := map[string]float64{
		"separate":   0.15,
		"parallel":   0.2,
		"concurrent": 0.2,
		"independent": 0.2,
		"isolated":   0.15,
	}

	score := 0.4 // baseline
	for keyword, weight := range independent {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	// Dependency indicators
	dependent := map[string]float64{
		"sequential": -0.15,
		"depends":    -0.15,
		"requires":   -0.1,
		"then":       -0.05,
	}
	for keyword, weight := range dependent {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	return math.Max(0, math.Min(1, score))
}

// assessSurfaceArea evaluates how many modules/directories are affected [0,1].
func (s *Scorer) assessSurfaceArea(task string) float64 {
	lower := strings.ToLower(task)

	// Count module mentions
	modules := []string{
		"auth", "api", "ui", "backend", "frontend", "database", "cache",
		"logging", "monitoring", "config", "utils", "core", "storage",
		"queue", "worker", "scheduler", "client", "server", "gateway",
	}

	count := 0
	for _, module := range modules {
		if strings.Contains(lower, module) {
			count++
		}
	}

	// Surface area score based on module count
	// More modules = higher surface area = potentially better for delegation
	return math.Min(1, float64(count)*0.15+0.2)
}

// assessCouplingRisk evaluates merge conflict and shared state risk [0,1].
func (s *Scorer) assessCouplingRisk(task string) float64 {
	lower := strings.ToLower(task)

	// High coupling indicators
	highCoupling := map[string]float64{
		"refactor":         0.2,
		"integration":      0.15,
		"dependency":       0.15,
		"shared":           0.15,
		"global":           0.2,
		"coupled":          0.2,
		"cross-module":     0.2,
		"cross-cutting":    0.25,
		"state management": 0.2,
	}

	score := 0.2 // baseline
	for keyword, weight := range highCoupling {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	return math.Max(0, math.Min(1, score))
}

// assessCognitiveLoad evaluates reasoning depth required [0,1].
func (s *Scorer) assessCognitiveLoad(task string) float64 {
	lower := strings.ToLower(task)

	// High complexity indicators
	complex := map[string]float64{
		"refactor":       0.15,
		"redesign":       0.2,
		"optimization":   0.15,
		"architecture":   0.2,
		"algorithm":      0.15,
		"implement":      0.1,
		"add":            0.05,
		"fix":            -0.1,
		"update":         -0.05,
	}

	score := 0.3 // baseline
	for keyword, weight := range complex {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	return math.Max(0, math.Min(1, score))
}

// assessParallelEfficiency evaluates expected speedup from concurrency [0,1].
func (s *Scorer) assessParallelEfficiency(task string) float64 {
	lower := strings.ToLower(task)

	// Parallelizable patterns
	parallel := map[string]float64{
		"multiple":    0.15,
		"several":     0.1,
		"different":   0.1,
		"separate":    0.15,
		"independent": 0.2,
		"parallel":    0.2,
		"concurrent":  0.2,
	}

	score := 0.3 // baseline
	for keyword, weight := range parallel {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	// Sequential patterns reduce efficiency
	sequential := map[string]float64{
		"then":       -0.1,
		"after":      -0.1,
		"before":     -0.1,
		"sequential": -0.2,
		"depends on": -0.15,
	}
	for keyword, weight := range sequential {
		if strings.Contains(lower, keyword) {
			score += weight
		}
	}

	return math.Max(0, math.Min(1, score))
}

// detectAntiPatterns identifies patterns that make delegation risky.
func (s *Scorer) detectAntiPatterns(task string, couplingRisk float64) []string {
	var patterns []string
	lower := strings.ToLower(task)

	antiPatterns := map[string]string{
		"global state":     "Global state mutation detected",
		"cross-cutting":    "Cross-cutting concerns (logging, config, etc.)",
		"monkey patch":     "Monkey patching or dynamic modifications",
		"tightly coupled":  "Tightly coupled refactoring",
		"shared interface": "Shared interfaces across modules",
	}

	for keyword, pattern := range antiPatterns {
		if strings.Contains(lower, keyword) {
			patterns = append(patterns, pattern)
		}
	}

	// High coupling risk is itself an anti-pattern
	if couplingRisk > 0.7 {
		patterns = append(patterns, "High merge conflict risk")
	}

	return patterns
}

// determineRecommendation maps score to action recommendation.
func (s *Scorer) determineRecommendation(score float64, antiPatterns []string, couplingRisk float64) DelegationRecommendation {
	if len(antiPatterns) > 0 && score > 0.65 {
		return RecommendWithCaution
	}
	if len(antiPatterns) > 0 && score <= 0.65 {
		return RecommendSingleAgent
	}
	if couplingRisk > 0.7 {
		return RecommendStagedExec
	}
	if score >= 0.6 {
		return RecommendDelegate
	}
	return RecommendSingleAgent
}

// computeConfidence evaluates confidence in the score [0,1].
func (s *Scorer) computeConfidence(task string, antiPatterns []string) float64 {
	confidence := 0.7 // baseline

	// More explicit task description = higher confidence
	if len(task) > 200 {
		confidence += 0.15
	} else if len(task) < 50 {
		confidence -= 0.2
	}

	// Anti-patterns reduce confidence
	if len(antiPatterns) > 0 {
		confidence -= 0.15 * float64(len(antiPatterns))
	}

	return math.Max(0, math.Min(1, confidence))
}

// generateReasoning creates a human-readable summary of the score.
func (s *Scorer) generateReasoning(score *DelegabilityScore) string {
	var sb strings.Builder

	// Summarize top factors
	factors := []struct {
		name  string
		value float64
	}{
		{"Modularity", score.Modularity},
		{"Independence", score.Independence},
		{"Surface Area", score.SurfaceArea},
		{"Parallel Efficiency", score.ParallelEfficiency},
		{"Cognitive Load", score.CognitiveLoad},
		{"Coupling Risk", score.CouplingRisk},
	}

	sb.WriteString(fmt.Sprintf("Delegatability: %.1f%% - %s. ", score.Score*100, score.Recommendation))

	// Top positive factors
	sb.WriteString("Positive: ")
	maxFactors := 2
	for i, f := range factors {
		if i >= maxFactors || f.value < 0.5 {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s (%.0f%%), ", f.name, f.value*100))
	}

	// Risk factors
	if score.CouplingRisk > 0.6 {
		sb.WriteString(fmt.Sprintf("Risk: High coupling potential (%.0f%%). ", score.CouplingRisk*100))
	}

	if len(score.AntiPatterns) > 0 {
		sb.WriteString(fmt.Sprintf("Caution: %s. ", strings.Join(score.AntiPatterns, ", ")))
	}

	return sb.String()
}

// suggestDecompositionStrategy recommends a decomposition method.
func (s *Scorer) suggestDecompositionStrategy(task string, score *DelegabilityScore) *DecompositionStrategy {
	lower := strings.ToLower(task)
	strategy := &DecompositionStrategy{}

	// Determine method based on task characteristics
	if strings.Contains(lower, "layer") || strings.Contains(lower, "architecture") {
		strategy.Method = "layer-based"
		strategy.Rationale = "Task involves distinct architectural layers"
	} else if strings.Contains(lower, "feature") {
		strategy.Method = "feature-sliced"
		strategy.Rationale = "Task can be decomposed by feature boundaries"
	} else {
		strategy.Method = "module-based"
		strategy.Rationale = "Task can be split along module boundaries"
	}

	// Estimate number of subtasks based on surface area
	strategy.EstimatedSubtasks = 2 + int(score.SurfaceArea*3)

	return strategy
}
