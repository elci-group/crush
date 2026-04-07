package delegation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParseDelegationPlanFromResponse extracts a DecompositionAnalysis from agent response text.
// Looks for JSON blocks (both ```json and bare JSON) containing DecompositionAnalysis.
func ParseDelegationPlanFromResponse(responseText string) (*DecompositionAnalysis, error) {
	if responseText == "" {
		return nil, fmt.Errorf("empty response text")
	}

	// Try to extract JSON from markdown code blocks first
	jsonBlocks := extractJSONBlocks(responseText)
	for _, block := range jsonBlocks {
		analysis, err := tryParseAnalysis(block)
		if err == nil && analysis != nil {
			return analysis, nil
		}
	}

	// Try to parse the whole response as JSON (in case it's bare JSON)
	analysis, err := tryParseAnalysis(responseText)
	if err == nil && analysis != nil {
		return analysis, nil
	}

	return nil, fmt.Errorf("no valid DecompositionAnalysis found in response")
}

// extractJSONBlocks extracts JSON content from markdown code blocks and JSON patterns.
func extractJSONBlocks(text string) []string {
	var blocks []string

	// Pattern for ```json ... ``` blocks
	jsonBlockRegex := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?([^`]+)```")
	matches := jsonBlockRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			blocks = append(blocks, match[1])
		}
	}

	// Pattern for { ... } JSON objects (at least 100 chars to avoid false positives)
	if len(blocks) == 0 {
		// Find the first { and try to find matching }
		startIdx := strings.Index(text, "{")
		if startIdx >= 0 {
			braceCount := 0
			for i := startIdx; i < len(text); i++ {
				if text[i] == '{' {
					braceCount++
				} else if text[i] == '}' {
					braceCount--
					if braceCount == 0 {
						potentialJSON := text[startIdx : i+1]
						if len(potentialJSON) > 100 {
							blocks = append(blocks, potentialJSON)
						}
						break
					}
				}
			}
		}
	}

	return blocks
}

// tryParseAnalysis attempts to parse a string as DecompositionAnalysis.
func tryParseAnalysis(jsonStr string) (*DecompositionAnalysis, error) {
	// Clean the JSON string
	jsonStr = strings.TrimSpace(jsonStr)

	var analysis DecompositionAnalysis
	err := json.Unmarshal([]byte(jsonStr), &analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate that we have a meaningful plan
	if !analysis.CanDecompose || analysis.ProposedPlan == nil || len(analysis.ProposedPlan.SubTasks) == 0 {
		return nil, fmt.Errorf("invalid analysis: no decomposable plan with sub-tasks")
	}

	return &analysis, nil
}

// ValidatePlan checks if a delegation plan is executable.
func ValidatePlan(plan *DelegationPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}

	if plan.ID == "" {
		return fmt.Errorf("plan has no ID")
	}

	if len(plan.SubTasks) == 0 {
		return fmt.Errorf("plan has no sub-tasks")
	}

	if plan.OriginalTask == "" {
		return fmt.Errorf("plan has no original task description")
	}

	// Validate each sub-task
	for _, task := range plan.SubTasks {
		if task.ID == "" {
			return fmt.Errorf("sub-task has no ID")
		}
		if task.AssignedModel.Model == "" {
			return fmt.Errorf("sub-task %s has no assigned model", task.ID)
		}
		if task.AssignedModel.Provider == "" {
			return fmt.Errorf("sub-task %s has no assigned provider", task.ID)
		}
		if len(task.Scope.Paths) == 0 {
			return fmt.Errorf("sub-task %s has no scope paths", task.ID)
		}
	}

	return nil
}
