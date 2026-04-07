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
		if result == nil {
			return nil
		}
		return result
	}
	return nil
}

// handleKeyMsg processes keyboard input and returns an action.
func (d *DelegationDialog) handleKeyMsg(msg tea.KeyPressMsg) Action {
	if d.Analysis == nil || d.Analysis.ProposedPlan == nil {
		return nil
	}

	// Handle key bindings
	switch {
	case key.Matches(msg, d.KeyApprove):
		d.Analysis.ProposedPlan.ApprovedBy = true
		return &ActionDelegationApproved{
			Plan: d.Analysis.ProposedPlan,
		}

	case key.Matches(msg, d.KeyReject):
		return &ActionDelegationRejected{
			Reason: "user rejected delegation plan",
		}

	case key.Matches(msg, d.KeyModify):
		d.modifyMode = !d.modifyMode
		return nil

	case key.Matches(msg, d.KeyUp):
		if d.selectedTask > 0 {
			d.selectedTask--
		}
		return nil

	case key.Matches(msg, d.KeyDown):
		plan := d.Analysis.ProposedPlan
		if d.selectedTask < len(plan.SubTasks)-1 {
			d.selectedTask++
		}
		return nil

	case key.Matches(msg, d.KeyEsc):
		return &ActionClose{}
	}

	// Handle text input in modify mode
	if d.modifyMode {
		ch := msg.String()
		if len(ch) == 1 && ch >= " " && ch <= "~" {
			d.modifications += ch
			d.Analysis.ProposedPlan.Modifications = d.modifications
			return nil
		}
	}

	return nil
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
	sb.WriteString(fmt.Sprintf("Scheduling %d agents in parallel:\n", len(plan.SubTasks)))
	sb.WriteString(strings.Repeat("─", width-2))
	sb.WriteString("\n\n")

	// Model assignment breakdown table
	sb.WriteString(d.renderModelAssignmentTable(plan, width))

	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width-2))
	sb.WriteString("\n\n")

	// Render selected task details
	if d.selectedTask < len(plan.SubTasks) {
		task := plan.SubTasks[d.selectedTask]
		sb.WriteString(d.renderTaskDetail(&task))
		sb.WriteString("\n\n")
		sb.WriteString(strings.Repeat("─", width-2))
		sb.WriteString("\n\n")
	}

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

// renderModelAssignmentTable displays model-to-task assignments in a table format.
func (d *DelegationDialog) renderModelAssignmentTable(plan *delegation.DelegationPlan, width int) string {
	var sb strings.Builder

	// Column widths (approximately)
	modelCol := 20
	taskCol := 25
	complexityCol := 4
	filesCol := width - modelCol - taskCol - complexityCol - 8

	if filesCol < 10 {
		filesCol = 10
	}

	// Header row
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("7")). // white
		Background(lipgloss.Color("8"))  // dark gray

	header := fmt.Sprintf("%-*s │ %-*s │ Cplx │ %-*s",
		modelCol, "Model",
		taskCol, "Task",
		filesCol, "Files")

	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")

	// Separator
	separator := strings.Repeat("─", modelCol) + "─┼─" + strings.Repeat("─", taskCol) + "─┼──────┼─" + strings.Repeat("─", filesCol)
	sepLen := min(len(separator), width)
	sb.WriteString(separator[:sepLen])
	sb.WriteString("\n")

	// Task rows
	for i, task := range plan.SubTasks {
		// Model info (provider + model name)
		modelStr := fmt.Sprintf("%s:%s", task.AssignedProvider, truncate(task.AssignedModel.Model, modelCol-len(task.AssignedProvider)-2))

		// Task title
		taskStr := truncate(task.Title, taskCol)

		// Complexity indicator
		complexityStr := fmt.Sprintf("%d/10", task.EstimatedComplexity)

		// File scope summary
		var filesStr string
		if len(task.Scope.Paths) > 0 {
			filesStr = fmt.Sprintf("%d patterns", len(task.Scope.Paths))
		} else {
			filesStr = "all"
		}
		filesStr = truncate(filesStr, filesCol)

		// Row with optional highlight if selected
		rowStyle := lipgloss.NewStyle()
		if i == d.selectedTask {
			rowStyle = rowStyle.
				Background(lipgloss.Color("4")).  // blue background
				Foreground(lipgloss.Color("15")) // white text
		}

		row := fmt.Sprintf("%-*s │ %-*s │ %4s │ %-*s",
			modelCol, modelStr,
			taskCol, taskStr,
			complexityStr,
			filesCol, filesStr)

		sb.WriteString(rowStyle.Render(row))
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncate shortens a string to max length with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 2 {
		return s[:maxLen-2] + ".."
	}
	return s[:maxLen]
}

// renderTaskDetail displays full details of a selected sub-task.
func (d *DelegationDialog) renderTaskDetail(task *delegation.SubTask) string {
	var sb strings.Builder

	// Title and ID
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14"))
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Task: %s [%s]", task.Title, task.ID)))
	sb.WriteString("\n\n")

	// Model assignment (highlighted)
	modelBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")). // magenta
		Padding(0, 1)

	modelInfo := fmt.Sprintf("Provider: %s  │  Model: %s  │  Branch: %s",
		task.AssignedProvider,
		task.AssignedModel.Model,
		task.BranchName)
	sb.WriteString(modelBoxStyle.Render(modelInfo))
	sb.WriteString("\n\n")

	// Complexity and scope summary
	complexityBar := d.renderComplexityBar(task.EstimatedComplexity)
	sb.WriteString(fmt.Sprintf("Complexity: %s\n", complexityBar))
	sb.WriteString(fmt.Sprintf("Scope: %s\n", task.Scope.Description))

	// File patterns
	if len(task.Scope.Paths) > 0 {
		sb.WriteString("\nFile Patterns:\n")
		for _, path := range task.Scope.Paths {
			sb.WriteString(fmt.Sprintf("  • %s\n", path))
		}
	}

	// Description
	sb.WriteString(fmt.Sprintf("\nTask Description:\n%s", indentLines(task.Description, 2)))

	return sb.String()
}

// renderComplexityBar renders a visual bar for complexity level
func (d *DelegationDialog) renderComplexityBar(complexity int) string {
	// Clamp complexity to 1-10
	if complexity < 1 {
		complexity = 1
	}
	if complexity > 10 {
		complexity = 10
	}

	filled := strings.Repeat("█", complexity)
	empty := strings.Repeat("░", 10-complexity)

	var color string
	if complexity <= 3 {
		color = "10" // green
	} else if complexity <= 6 {
		color = "11" // yellow
	} else {
		color = "9" // red
	}

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	return fmt.Sprintf("[%s] %d/10", barStyle.Render(filled+empty), complexity)
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
