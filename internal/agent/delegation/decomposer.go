package delegation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/config"
)

// Decomposer analyzes tasks and creates delegation plans.
type Decomposer struct {
	cfg *config.Config
}

// NewDecomposer creates a new task decomposer.
func NewDecomposer(cfg *config.Config) *Decomposer {
	return &Decomposer{cfg: cfg}
}

// NewDecomposerFromStore creates a new task decomposer from a ConfigStore.
func NewDecomposerFromStore(cfgStore *config.ConfigStore) *Decomposer {
	return &Decomposer{cfg: cfgStore.Config()}
}

// AnalyzeTask examines a task to determine if it should be delegated.
// Returns a DecompositionAnalysis with proposed delegation plan if suitable.
func (d *Decomposer) AnalyzeTask(ctx context.Context, taskDescription string) (*DecompositionAnalysis, error) {
	analysis := &DecompositionAnalysis{
		CanDecompose: false,
		Reason:       "",
		Confidence:   0,
	}

	// Check complexity indicators
	complexityScore := d.assessComplexity(taskDescription)
	if complexityScore < 5 {
		analysis.Reason = fmt.Sprintf("Task complexity is low (%d/10) - agent can handle solo without delegation", complexityScore)
		return analysis, nil
	}

	// Check if multiple independent modules/features are involved
	modules := d.identifyModules(taskDescription)
	if len(modules) < 2 {
		analysis.Reason = fmt.Sprintf("Task targets single module (%v) - delegation adds overhead without benefit", modules)
		return analysis, nil
	}

	// Check available models
	availableModels := d.getAvailableModels()
	if len(availableModels) < 2 {
		analysis.Reason = fmt.Sprintf("Only %d models available (need 2+) - delegation requires multiple providers", len(availableModels))
		return analysis, nil
	}

	// Task is suitable for delegation - create plan
	analysis.CanDecompose = true
	analysis.Confidence = min(95, 60+complexityScore)
	analysis.Reason = fmt.Sprintf("Task is complex (%d/10) and spans %d independent modules - well-suited for parallel delegation", complexityScore, len(modules))

	// Generate delegation plan
	plan := d.createPlan(taskDescription, modules, availableModels)
	analysis.ProposedPlan = plan

	// Could generate alternative plans here for user choice
	// For now, just one plan

	return analysis, nil
}

// assessComplexity returns a score 1-10 based on task description indicators.
func (d *Decomposer) assessComplexity(task string) int {
	score := 3 // baseline

	// Complexity indicators
	indicators := map[string]int{
		"refactor":       +2,
		"redesign":       +3,
		"multi":          +2,
		"integrate":      +2,
		"multiple":       +1,
		"migrate":        +2,
		"optimization":   +1,
		"parallel":       +2,
		"concurrent":     +2,
		"distributed":    +3,
		"architecture":   +3,
		"framework":      +2,
		"system":         +2,
		"complex":        +2,
		"components":     +1,
		"modules":        +1,
		"apis":           +1,
		"services":       +2,
	}

	lower := strings.ToLower(task)
	for keyword, points := range indicators {
		if strings.Contains(lower, keyword) {
			score += points
		}
	}

	// Subtract if simple indicators
	simpleIndicators := map[string]int{
		"fix":        -1,
		"typo":       -2,
		"small":      -2,
		"trivial":    -3,
		"minor":      -1,
	}

	for keyword, penalty := range simpleIndicators {
		if strings.Contains(lower, keyword) {
			score += penalty
		}
	}

	return max(1, min(10, score))
}

// identifyModules extracts likely module/component names from task description.
func (d *Decomposer) identifyModules(task string) []string {
	// Common architectural terms
	terms := []string{
		"auth", "api", "ui", "backend", "frontend", "database", "cache",
		"logging", "monitoring", "config", "utils", "core", "storage",
		"queue", "worker", "scheduler", "client", "server", "gateway",
	}

	var found []string
	lower := strings.ToLower(task)

	for _, term := range terms {
		if strings.Contains(lower, term) {
			found = append(found, term)
		}
	}

	// If no terms found, extract words in quotes or after "module/component"
	if len(found) == 0 {
		words := strings.Fields(task)
		// Take first few significant words as modules
		for i, w := range words {
			if i < 3 && len(w) > 4 {
				found = append(found, strings.ToLower(w))
			}
		}
	}

	return found
}

// getAvailableModels returns models the user has API keys for.
// Returns the user's configured large and small models as available.
func (d *Decomposer) getAvailableModels() []config.SelectedModel {
	var available []config.SelectedModel

	// Add configured large model if it exists
	if largeModel, ok := d.cfg.Models[config.SelectedModelTypeLarge]; ok && largeModel.Provider != "" {
		available = append(available, largeModel)
	}

	// Add configured small model if it exists
	if smallModel, ok := d.cfg.Models[config.SelectedModelTypeSmall]; ok && smallModel.Provider != "" {
		available = append(available, smallModel)
	}

	// Check for additional configured models in the Models map
	// (Additional models beyond large/small can be added here if needed)
	// For now, we use the two primary model selections

	return available
}

// createPlan generates a delegation plan for the task.
func (d *Decomposer) createPlan(taskDescription string, modules []string, availableModels []config.SelectedModel) *DelegationPlan {
	plan := &DelegationPlan{
		ID:           generatePlanID(),
		OriginalTask: taskDescription,
		CreatedAt:    time.Now(),
		Dependencies: make(map[string][]string),
	}

	// Distribute modules across available models
	numTasks := min(len(modules), len(availableModels))

	for i := 0; i < numTasks; i++ {
		module := modules[i]
		model := availableModels[i]

		taskID := fmt.Sprintf("task_%d", i+1)
		subTask := SubTask{
			ID:                  taskID,
			Title:               fmt.Sprintf("%s module: %s", strings.ToUpper(module[:1])+module[1:], strings.ToTitle(module)),
			Description:         fmt.Sprintf("Implement/fix %s functionality: %s", module, taskDescription),
			AssignedModel:       model,
			AssignedProvider:    model.Provider,
			EstimatedComplexity: 5 + (i % 3), // vary complexity
			Status:              SubTaskPending,
			BranchName:          fmt.Sprintf("feature/%s-%s", module, fmt.Sprintf("%d", time.Now().Unix()%10000)),
			CreatedAt:           time.Now(),
		}

		// Set scope
		subTask.Scope = SubTaskScope{
			Paths:       []string{fmt.Sprintf("**/*%s*", module), fmt.Sprintf("**/%s/**", module)},
			Description: fmt.Sprintf("Files related to %s module", module),
		}

		plan.SubTasks = append(plan.SubTasks, subTask)
	}

	// Set rationale
	plan.Rationale = fmt.Sprintf(
		"Complex task spans %d independent modules (%s). Assigning each to a specialized model enables parallel development with clear boundaries to avoid conflicts.",
		len(plan.SubTasks),
		strings.Join(modules[:len(plan.SubTasks)], ", "),
	)

	return plan
}

func generatePlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().Unix())
}

// ValidatePlan checks if a plan is executable.
func (d *Decomposer) ValidatePlan(plan *DelegationPlan) (bool, []string) {
	var issues []string

	if len(plan.SubTasks) == 0 {
		issues = append(issues, "plan has no sub-tasks")
		return false, issues
	}

	if len(plan.SubTasks) > len(d.getAvailableModels()) {
		issues = append(issues, fmt.Sprintf("plan requires %d agents but only %d models available", len(plan.SubTasks), len(d.getAvailableModels())))
	}

	// Check for scope conflicts
	seenPaths := make(map[string]string)
	for _, task := range plan.SubTasks {
		for _, path := range task.Scope.Paths {
			if prev, exists := seenPaths[path]; exists && prev != task.ID {
				issues = append(issues, fmt.Sprintf("scope conflict: path '%s' assigned to both %s and %s", path, prev, task.ID))
			}
			seenPaths[path] = task.ID
		}
	}

	return len(issues) == 0, issues
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
