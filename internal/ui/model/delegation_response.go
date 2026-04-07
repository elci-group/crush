package model

import (
	"fmt"
	"log/slog"

	"github.com/charmbracelet/crush/internal/agent/delegation"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/ui/dialog"
)

// CheckForDelegationPlanInMessage examines a message for embedded delegation plans.
// If found, extracts and prepares the plan for execution.
func (m *UI) CheckForDelegationPlanInMessage(msg *message.Message) bool {
	if msg == nil {
		return false
	}

	// Get the text content from the message
	content := msg.Content()
	text := content.Text
	if text == "" {
		return false
	}

	// Try to parse a delegation plan from the response
	analysis, err := delegation.ParseDelegationPlanFromResponse(text)
	if err != nil {
		// No plan found, not an error - just a regular message
		return false
	}

	// Validate the plan
	if err := delegation.ValidatePlan(analysis.ProposedPlan); err != nil {
		slog.Warn("Invalid delegation plan in agent response", "error", err)
		return false
	}

	slog.Info("Delegation plan detected in agent response",
		"plan_id", analysis.ProposedPlan.ID,
		"sub_tasks", len(analysis.ProposedPlan.SubTasks),
		"confidence", analysis.Confidence)

	// Store the detected plan and analysis for user approval
	m.delegationPlan = analysis.ProposedPlan
	m.delegationAnalysis = analysis

	// Show the delegation dialog for user approval
	m.showDelegationPlanDialog(analysis)

	return true
}

// showDelegationPlanDialog displays the delegation plan for user approval.
func (m *UI) showDelegationPlanDialog(analysis *delegation.DecompositionAnalysis) {
	if m.dialog.ContainsDialog(dialog.DelegationID) {
		m.dialog.BringToFront(dialog.DelegationID)
		return
	}

	delegationDialog := dialog.NewDelegationDialog(analysis)
	m.dialog.OpenDialog(delegationDialog)

	// Log the plan details for debugging
	if analysis.ProposedPlan != nil {
		slog.Debug("Showing delegation plan dialog",
			"confidence", analysis.Confidence,
			"sub_tasks", len(analysis.ProposedPlan.SubTasks))

		for i, task := range analysis.ProposedPlan.SubTasks {
			slog.Debug("Sub-task details",
				"index", i+1,
				"id", task.ID,
				"title", task.Title,
				"model", task.AssignedModel.Model,
				"provider", task.AssignedModel.Provider,
				"complexity", task.EstimatedComplexity)
		}
	}
}

// NotifyDelegationPlanExtracted notifies the user that a plan was found and extracted.
func (m *UI) NotifyDelegationPlanExtracted() {
	if m.delegationPlan != nil {
		msg := fmt.Sprintf("📋 Delegation plan extracted: %d agents ready to execute. Press enter to approve or r to reject.",
			len(m.delegationPlan.SubTasks))
		// This notification could be displayed as a UI element or logged
		slog.Info("Plan extraction notification", "message", msg)
	}
}
