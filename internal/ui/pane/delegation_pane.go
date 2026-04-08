package pane

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/agent/delegation"
	uv "github.com/charmbracelet/ultraviolet"
)

// DelegationPane manages the delegation planning UI as a modal overlay.
type DelegationPane struct {
	Width  int
	Height int

	Analysis  *delegation.DecompositionAnalysis
	Selecting int // Currently selected sub-task

	Focused bool
}

// NewDelegationPane creates a new delegation planning pane.
func NewDelegationPane(analysis *delegation.DecompositionAnalysis) *DelegationPane {
	return &DelegationPane{
		Analysis:  analysis,
		Selecting: 0,
		Focused:   true,
	}
}

// Update handles messages for the delegation pane.
func (d *DelegationPane) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.Width = msg.Width
		d.Height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if d.Selecting > 0 {
				d.Selecting--
			}
		case "down", "j":
			if d.Analysis != nil && d.Selecting < len(d.Analysis.ProposedPlan.SubTasks)-1 {
				d.Selecting++
			}
		}
	}
	return nil
}

// Render returns the delegation pane UI.
func (d *DelegationPane) Render(width, height int) string {
	if d.Analysis == nil || d.Analysis.ProposedPlan == nil {
		return "No delegation plan"
	}

	plan := d.Analysis.ProposedPlan

	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")).
		Padding(1, 2)
	sb.WriteString(headerStyle.Render("📋 Delegation Plan"))
	sb.WriteString("\n\n")

	// Confidence and rationale
	confStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Padding(0, 2)
	sb.WriteString(confStyle.Render(fmt.Sprintf("Confidence: %d%% | %s", d.Analysis.Confidence, d.Analysis.Reason)))
	sb.WriteString("\n\n")

	// Sub-tasks list
	sb.WriteString(lipgloss.NewStyle().Padding(0, 2).Render("Sub-Tasks:"))
	sb.WriteString("\n")

	for i, task := range plan.SubTasks {
		prefix := "  □ "
		if i == d.Selecting {
			prefix = "  ▶ "
		}

		taskLine := fmt.Sprintf("%s%s (%s:%s) [Complexity: %d/10]",
			prefix,
			task.Title,
			task.AssignedModel.Provider,
			task.AssignedModel.Model,
			task.EstimatedComplexity)

		if i == d.Selecting {
			// Highlight selected task
			selectStyle := lipgloss.NewStyle().
				Background(lipgloss.Color("4")).
				Foreground(lipgloss.Color("15")).
				Padding(0, 1)
			sb.WriteString(selectStyle.Render(taskLine))
		} else {
			sb.WriteString("  ")
			sb.WriteString(taskLine)
		}
		sb.WriteString("\n")
	}

	// Selected task details
	if d.Selecting < len(plan.SubTasks) {
		task := plan.SubTasks[d.Selecting]
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat("─", 60))
		sb.WriteString("\n\n")

		detailStyle := lipgloss.NewStyle().Padding(0, 2)
		sb.WriteString(detailStyle.Render("📌 Task Details:"))
		sb.WriteString("\n")
		sb.WriteString(detailStyle.Render(fmt.Sprintf("Title: %s", task.Title)))
		sb.WriteString("\n")
		sb.WriteString(detailStyle.Render(fmt.Sprintf("Model: %s (%s)", task.AssignedModel.Model, task.AssignedModel.Provider)))
		sb.WriteString("\n")
		sb.WriteString(detailStyle.Render(fmt.Sprintf("Complexity: %d/10", task.EstimatedComplexity)))
		sb.WriteString("\n")
		sb.WriteString(detailStyle.Render(fmt.Sprintf("Scope: %s", task.Scope.Description)))
		sb.WriteString("\n\n")
		sb.WriteString(detailStyle.Render(fmt.Sprintf("Description:\n%s", task.Description)))
	}

	sb.WriteString("\n\n")
	sb.WriteString(strings.Repeat("─", 60))
	sb.WriteString("\n")

	// Controls
	controlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 2)
	sb.WriteString(controlStyle.Render("↑↓ navigate  •  enter approve  •  r reject  •  esc cancel"))

	return sb.String()
}

// Draw renders the pane to screen as a centered modal.
func (d *DelegationPane) Draw(scr uv.Screen, area uv.Rectangle) {
	content := d.Render(area.Dx(), area.Dy())

	// Create modal box
	modalWidth := min(area.Dx()-4, 80)
	modalHeight := min(area.Dy()-4, 30)

	// Center the modal
	x := (area.Dx() - modalWidth) / 2
	y := (area.Dy() - modalHeight) / 2

	// Draw semi-transparent overlay background
	overlayStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Padding(0)

	// Draw modal box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1).
		Background(lipgloss.Color("8"))

	styledContent := uv.NewStyledString(boxStyle.Render(content))
	rect := uv.NewRectangle(x, y, modalWidth, modalHeight)
	styledContent.Draw(scr, rect)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
