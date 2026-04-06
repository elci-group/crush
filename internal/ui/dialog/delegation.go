package dialog

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/agent/delegation"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	DelegationID = "delegation"
)

// DelegationDialog manages task decomposition plan review and approval.
type DelegationDialog struct {
	Analysis *delegation.DecompositionAnalysis
	Focused  bool

	// UI state
	selectedTask  int // which sub-task is selected for details
	modifyMode    bool // editing modifications
	modifications string

	// Keybindings
	KeyApprove key.Binding
	KeyReject  key.Binding
	KeyModify  key.Binding
	KeyUp      key.Binding
	KeyDown    key.Binding
	KeyEsc     key.Binding
}

// NewDelegationDialog creates a delegation plan review dialog.
// This is exported so it can be called from the model package.
func NewDelegationDialog(analysis *delegation.DecompositionAnalysis) *DelegationDialog {
	return &DelegationDialog{
		Analysis:     analysis,
		selectedTask: 0,
		KeyApprove:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "approve plan")),
		KeyReject:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reject plan")),
		KeyModify:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "add modifications")),
		KeyUp:        key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous task")),
		KeyDown:      key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "next task")),
		KeyEsc:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
}

// ID returns the dialog identifier.
func (d *DelegationDialog) ID() string {
	return DelegationID
}

// IsFocused returns whether the dialog has focus.
func (d *DelegationDialog) IsFocused() bool {
	return d.Focused
}

// SetFocused sets focus state.
func (d *DelegationDialog) SetFocused(focused bool) {
	d.Focused = focused
}

// HandleMsg processes a BubbleTea message and returns an action.
func (d *DelegationDialog) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		result := d.handleKeyMsg(msg)
		if result == "" {
			return nil
		}
		return result
	}
	return nil
}

// handleKeyMsg processes keyboard input and returns action string.
func (d *DelegationDialog) handleKeyMsg(msg tea.KeyPressMsg) string {
	if d.Analysis == nil || d.Analysis.ProposedPlan == nil {
		return ""
	}

	// Handle key bindings
	switch {
	case key.Matches(msg, d.KeyApprove):
		d.Analysis.ProposedPlan.ApprovedBy = true
		return "delegation_approved"

	case key.Matches(msg, d.KeyReject):
		return "delegation_rejected"

	case key.Matches(msg, d.KeyModify):
		d.modifyMode = !d.modifyMode
		return "modified"

	case key.Matches(msg, d.KeyUp):
		if d.selectedTask > 0 {
			d.selectedTask--
		}
		return "selection_changed"

	case key.Matches(msg, d.KeyDown):
		plan := d.Analysis.ProposedPlan
		if d.selectedTask < len(plan.SubTasks)-1 {
			d.selectedTask++
		}
		return "selection_changed"

	case key.Matches(msg, d.KeyEsc):
		return "dialog_close"
	}

	// Handle text input in modify mode
	if d.modifyMode {
		ch := msg.String()
		if len(ch) == 1 && ch >= " " && ch <= "~" {
			d.modifications += ch
			d.Analysis.ProposedPlan.Modifications = d.modifications
			return "modifications_updated"
		}
	}

	return ""
}

// Draw renders the dialog onto the screen.
func (d *DelegationDialog) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	// Render the dialog content
	content := d.Render(area.Dx(), area.Dy())
	styledStr := uv.NewStyledString(content)
	styledStr.Draw(scr, area)
	return nil
}

// Render returns the dialog UI.
func (d *DelegationDialog) Render(width, height int) string {
	if d.Analysis == nil || !d.Analysis.CanDecompose {
		return d.renderNotSuitable(width, height)
	}

	plan := d.Analysis.ProposedPlan
	if plan == nil || len(plan.SubTasks) == 0 {
		return d.renderNotSuitable(width, height)
	}

	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")). // cyan
		Padding(0, 1)
	sb.WriteString(headerStyle.Render("Task Delegation Plan"))
	sb.WriteString("\n\n")

	// Confidence and rationale
	confStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	sb.WriteString(confStyle.Render(fmt.Sprintf("Confidence: %d%%", d.Analysis.Confidence)))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Rationale: %s\n\n", d.Analysis.Reason))

	// Sub-tasks overview
	sb.WriteString("Sub-tasks: ")
	sb.WriteString(fmt.Sprintf("%d parallel modules\n", len(plan.SubTasks)))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n\n")

	// Render selected task details
	if d.selectedTask < len(plan.SubTasks) {
		task := plan.SubTasks[d.selectedTask]
		sb.WriteString(d.renderTaskDetail(&task))
	}

	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n\n")

	// Task list
	sb.WriteString("Tasks:\n")
	for i, task := range plan.SubTasks {
		prefix := "  "
		if i == d.selectedTask {
			prefix = "▶ "
		}
		sb.WriteString(fmt.Sprintf("%s[%d] %s (%s)\n", prefix, i+1, task.Title, task.AssignedProvider))
	}

	sb.WriteString("\n")

	// Modifications input
	if d.modifyMode {
		inputStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			BorderForeground(lipgloss.Color("3")) // yellow
		sb.WriteString(inputStyle.Render(fmt.Sprintf("Modifications: %s|", d.modifications)))
		sb.WriteString("\n\n")
	}

	// Status/help
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	help := []string{
		"↑↓ select task",
		"m modify",
		"enter approve",
		"r reject",
		"esc close",
	}
	sb.WriteString(helpStyle.Render(strings.Join(help, " • ")))

	return sb.String()
}

// renderTaskDetail displays full details of a selected sub-task.
func (d *DelegationDialog) renderTaskDetail(task *delegation.SubTask) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Task: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("Provider: %s\n", task.AssignedProvider))
	sb.WriteString(fmt.Sprintf("Model: %s\n", task.AssignedModel.Model))
	sb.WriteString(fmt.Sprintf("Complexity: %d/10\n", task.EstimatedComplexity))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", task.BranchName))
	sb.WriteString(fmt.Sprintf("Scope: %s\n", task.Scope.Description))

	if len(task.Scope.Paths) > 0 {
		sb.WriteString("Files: ")
		sb.WriteString(strings.Join(task.Scope.Paths, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("\nDescription:\n%s", indentLines(task.Description, 2)))

	return sb.String()
}

// renderNotSuitable shows why task cannot be decomposed.
func (d *DelegationDialog) renderNotSuitable(width, height int) string {
	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("1"))
	sb.WriteString(headerStyle.Render("Task Cannot Be Delegated"))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("Reason: %s\n", d.Analysis.Reason))
	sb.WriteString("\nThis task is better handled as a single unified effort.\n")

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sb.WriteString(helpStyle.Render("press esc to close"))

	return sb.String()
}

// indentLines prepends spaces to each line.
func indentLines(text string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}
