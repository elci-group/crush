package model

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/agent/delegation"
	"github.com/charmbracelet/crush/internal/ui/dialog"
)

// AnalyzeDelegation analyzes the current task for delegation suitability.
// It takes the user's input text and determines if it should be decomposed.
func (ui *UI) AnalyzeDelegation(ctx context.Context) error {
	if ui.textarea.Value() == "" {
		return fmt.Errorf("no task description provided")
	}

	taskDescription := ui.textarea.Value()

	// Create decomposer and analyze task
	decomposer := delegation.NewDecomposer(ui.com.Config())
	analysis, err := decomposer.AnalyzeTask(ctx, taskDescription)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Create delegation dialog and show it
	dlg := dialog.NewDelegationDialog(analysis)
	ui.dialog.OpenDialog(dlg)

	return nil
}

// StartDelegation starts execution of an approved delegation plan.
func (ui *UI) StartDelegation(plan *delegation.DelegationPlan) error {
	if ui.delegation == nil {
		return fmt.Errorf("delegation state not initialized")
	}

	// Start delegation UI state
	if err := ui.delegation.StartDelegation(ui.com, plan); err != nil {
		return fmt.Errorf("failed to start delegation: %w", err)
	}

	// Create sub-agent windows in the chat area
	coordinator := ui.delegation.Coordinator
	for _, task := range plan.SubTasks {
		// Start the agent in the coordinator
		if err := coordinator.StartAgent(&task); err != nil {
			return fmt.Errorf("failed to start agent for task %s: %w", task.ID, err)
		}
	}

	return nil
}

// EndDelegation stops delegation mode and cleans up.
func (ui *UI) EndDelegation() {
	if ui.delegation != nil {
		ui.delegation.EndDelegation()
	}
}

// GetDelegationStatus returns current delegation execution status.
func (ui *UI) GetDelegationStatus() *delegation.DelegationStatus {
	if ui.delegation == nil || !ui.delegation.IsActive() {
		return nil
	}

	status := ui.delegation.Coordinator.GetStatus()
	return &status
}

