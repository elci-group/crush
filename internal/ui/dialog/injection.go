package dialog

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/snapshot"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// InjectionID is the identifier for the injection control dialog.
const InjectionID = "injection"

const (
	injectionMaxWidth  = 110
	injectionMaxHeight = 35
	injTreeMinWidth    = 35
	injPreviewMinWidth = 30
	injTreePaneRatio   = 0.45
)

// injectionPane tracks which pane has focus.
type injectionPane int

const (
	injPaneTree injectionPane = iota
	injPanePreview
)

// ActionInjectionApply is emitted when the user applies their scope selection.
type ActionInjectionApply struct {
	Context *snapshot.CompiledContext
}

// Injection is the context injection control dialog.
type Injection struct {
	com *common.Common

	snap   *snapshot.Snapshot
	scope  *snapshot.ScopeSelection
	budget snapshot.TokenBudget

	// Tree state
	flatNodes []snapshot.FlatNode
	expanded  map[string]bool
	cursor    int
	treeOff   int // scroll offset

	// Preview state
	compiled   *snapshot.CompiledContext
	previewOff int

	pane   injectionPane
	help   help.Model
	keyMap injectionKeyMap
}

type injectionKeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Toggle       key.Binding
	Expand       key.Binding
	ExpandAll    key.Binding
	CollapseAll  key.Binding
	SelectAll    key.Binding
	ClearAll     key.Binding
	ToggleIgnore key.Binding
	SwitchPane   key.Binding
	ScrollUp     key.Binding
	ScrollDown   key.Binding
	Apply        key.Binding
	Close        key.Binding
}

var _ Dialog = (*Injection)(nil)

// NewInjection creates a new Injection dialog.
func NewInjection(com *common.Common, snap *snapshot.Snapshot) *Injection {
	t := com.Styles

	scope := snapshot.NewScopeSelection(snap.Root)

	expanded := make(map[string]bool)
	// Expand root children by default
	if snap.Tree != nil {
		for _, child := range snap.Tree.Children {
			if child.IsDir {
				expanded[child.Path] = true
			}
		}
	}

	h := help.New()
	h.Styles = t.DialogHelpStyles()

	inj := &Injection{
		com:      com,
		snap:     snap,
		scope:    scope,
		budget:   snapshot.DefaultBudget(),
		expanded: expanded,
		help:     h,
		keyMap: injectionKeyMap{
			Up: key.NewBinding(
				key.WithKeys("up", "k"),
				key.WithHelp("↑/k", "up"),
			),
			Down: key.NewBinding(
				key.WithKeys("down", "j"),
				key.WithHelp("↓/j", "down"),
			),
			Toggle: key.NewBinding(
				key.WithKeys(" "),
				key.WithHelp("space", "toggle"),
			),
			Expand: key.NewBinding(
				key.WithKeys("enter", "right", "l"),
				key.WithHelp("enter", "expand/collapse"),
			),
			ExpandAll: key.NewBinding(
				key.WithKeys("E"),
				key.WithHelp("E", "expand all"),
			),
			CollapseAll: key.NewBinding(
				key.WithKeys("W"),
				key.WithHelp("W", "collapse all"),
			),
			SelectAll: key.NewBinding(
				key.WithKeys("a"),
				key.WithHelp("a", "select all"),
			),
			ClearAll: key.NewBinding(
				key.WithKeys("A"),
				key.WithHelp("A", "clear all"),
			),
			ToggleIgnore: key.NewBinding(
				key.WithKeys("i"),
				key.WithHelp("i", "ignore"),
			),
			SwitchPane: key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("tab", "switch pane"),
			),
			ScrollUp: key.NewBinding(
				key.WithKeys("shift+up", "K"),
				key.WithHelp("shift+↑", "scroll preview"),
			),
			ScrollDown: key.NewBinding(
				key.WithKeys("shift+down", "J"),
				key.WithHelp("shift+↓", "scroll preview"),
			),
			Apply: key.NewBinding(
				key.WithKeys("ctrl+a"),
				key.WithHelp("ctrl+a", "apply"),
			),
			Close: CloseKey,
		},
	}

	inj.rebuildFlat()
	inj.recompile()

	return inj
}

// ID implements Dialog.
func (inj *Injection) ID() string {
	return InjectionID
}

// HandleMsg implements Dialog.
func (inj *Injection) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return inj.handleKey(msg)
	}
	return nil
}

func (inj *Injection) handleKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, inj.keyMap.Close):
		return ActionClose{}

	case key.Matches(msg, inj.keyMap.Apply):
		if inj.compiled != nil && len(inj.compiled.FileContents) > 0 {
			return ActionInjectionApply{Context: inj.compiled}
		}
		return ActionClose{}

	case key.Matches(msg, inj.keyMap.SwitchPane):
		if inj.pane == injPaneTree {
			inj.pane = injPanePreview
		} else {
			inj.pane = injPaneTree
		}

	case key.Matches(msg, inj.keyMap.Up):
		if inj.pane == injPaneTree {
			if inj.cursor > 0 {
				inj.cursor--
				inj.ensureTreeVisible()
			}
		} else {
			if inj.previewOff > 0 {
				inj.previewOff--
			}
		}

	case key.Matches(msg, inj.keyMap.Down):
		if inj.pane == injPaneTree {
			if inj.cursor < len(inj.flatNodes)-1 {
				inj.cursor++
				inj.ensureTreeVisible()
			}
		} else {
			inj.previewOff++
		}

	case key.Matches(msg, inj.keyMap.ScrollUp):
		if inj.previewOff > 0 {
			inj.previewOff--
		}

	case key.Matches(msg, inj.keyMap.ScrollDown):
		inj.previewOff++

	case key.Matches(msg, inj.keyMap.Toggle):
		if inj.cursor < len(inj.flatNodes) {
			node := inj.flatNodes[inj.cursor].Node
			if node.IsDir {
				// Toggle all files under this directory
				inj.toggleDir(node)
			} else {
				inj.scope.Toggle(node.Path)
			}
			inj.recompile()
		}

	case key.Matches(msg, inj.keyMap.Expand):
		if inj.cursor < len(inj.flatNodes) {
			node := inj.flatNodes[inj.cursor].Node
			if node.IsDir {
				inj.expanded[node.Path] = !inj.expanded[node.Path]
				inj.rebuildFlat()
			}
		}

	case key.Matches(msg, inj.keyMap.ExpandAll):
		inj.expandAll(inj.snap.Tree)
		inj.rebuildFlat()

	case key.Matches(msg, inj.keyMap.CollapseAll):
		inj.expanded = make(map[string]bool)
		inj.rebuildFlat()
		inj.cursor = 0
		inj.treeOff = 0

	case key.Matches(msg, inj.keyMap.SelectAll):
		inj.scope.SelectAll(inj.snap)
		inj.recompile()

	case key.Matches(msg, inj.keyMap.ClearAll):
		inj.scope.ClearSelection()
		inj.recompile()

	case key.Matches(msg, inj.keyMap.ToggleIgnore):
		if inj.cursor < len(inj.flatNodes) {
			node := inj.flatNodes[inj.cursor].Node
			wasIgnored := inj.scope.IsIgnored(node.Path)
			inj.scope.SetIgnored(node.Path, !wasIgnored)
			// If ignoring a dir, also ignore all children
			if node.IsDir {
				inj.setIgnoreRecursive(node, !wasIgnored)
			}
			inj.recompile()
		}
	}

	return nil
}

// toggleDir toggles selection on all files under a directory node.
func (inj *Injection) toggleDir(node *snapshot.TreeNode) {
	// Check if any child is selected -> if so, deselect all; otherwise select all
	anySelected := inj.anyChildSelected(node)
	inj.setDirSelection(node, !anySelected)
}

func (inj *Injection) anyChildSelected(node *snapshot.TreeNode) bool {
	for _, child := range node.Children {
		if child.IsDir {
			if inj.anyChildSelected(child) {
				return true
			}
		} else if inj.scope.IsSelected(child.Path) {
			return true
		}
	}
	return false
}

func (inj *Injection) setDirSelection(node *snapshot.TreeNode, selected bool) {
	for _, child := range node.Children {
		if child.IsDir {
			inj.setDirSelection(child, selected)
		} else if !inj.scope.IsIgnored(child.Path) {
			if selected {
				if !inj.scope.IsSelected(child.Path) {
					inj.scope.Toggle(child.Path)
				}
			} else {
				if inj.scope.IsSelected(child.Path) {
					inj.scope.Toggle(child.Path)
				}
			}
		}
	}
}

func (inj *Injection) setIgnoreRecursive(node *snapshot.TreeNode, ignored bool) {
	for _, child := range node.Children {
		inj.scope.SetIgnored(child.Path, ignored)
		if child.IsDir {
			inj.setIgnoreRecursive(child, ignored)
		}
	}
}

func (inj *Injection) expandAll(node *snapshot.TreeNode) {
	if node == nil {
		return
	}
	for _, child := range node.Children {
		if child.IsDir {
			inj.expanded[child.Path] = true
			inj.expandAll(child)
		}
	}
}

func (inj *Injection) rebuildFlat() {
	inj.flatNodes = snapshot.FlattenTree(inj.snap.Tree, inj.expanded)
	if inj.cursor >= len(inj.flatNodes) {
		inj.cursor = max(0, len(inj.flatNodes)-1)
	}
}

func (inj *Injection) recompile() {
	compiled, err := snapshot.Compile(inj.snap, inj.scope, inj.budget)
	if err != nil {
		return
	}
	inj.compiled = compiled
	inj.previewOff = 0
}

func (inj *Injection) ensureTreeVisible() {
	// Keep cursor in view with a 2-line margin
	if inj.cursor < inj.treeOff+2 {
		inj.treeOff = max(0, inj.cursor-2)
	}
}

// Cursor implements Dialog.
func (inj *Injection) Cursor() *tea.Cursor {
	return nil
}

// Draw implements Dialog.
func (inj *Injection) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := inj.com.Styles
	dialogStyle := t.Dialog.View

	totalWidth := min(injectionMaxWidth, area.Dx()-dialogStyle.GetHorizontalBorderSize())
	innerWidth := totalWidth - dialogStyle.GetHorizontalFrameSize()
	totalHeight := min(injectionMaxHeight, area.Dy()-2)
	innerHeight := totalHeight - dialogStyle.GetVerticalFrameSize() - 4 // title + help + status

	// Split into tree and preview panes
	treeWidth := int(float64(innerWidth) * injTreePaneRatio)
	if treeWidth < injTreeMinWidth {
		treeWidth = min(injTreeMinWidth, innerWidth)
	}
	previewWidth := innerWidth - treeWidth - 1 // 1 for separator

	rc := NewRenderContext(t, totalWidth)
	rc.Title = "Context Injection"

	// Status bar: selected count, token estimate, snapshot ID
	statusParts := []string{
		fmt.Sprintf("%d selected", inj.scope.SelectedCount()),
	}
	if inj.compiled != nil {
		statusParts = append(statusParts, fmt.Sprintf("~%dk tokens", inj.compiled.EstTokens/1000))
	}
	statusParts = append(statusParts, fmt.Sprintf("snap:%s", truncID(inj.snap.ID)))
	rc.TitleInfo = " " + t.Subtle.Render(strings.Join(statusParts, " | "))

	// Render tree pane
	treeView := inj.renderTree(t, treeWidth, innerHeight)

	// Render preview pane
	previewView := inj.renderPreview(t, previewWidth, innerHeight)

	// Separator
	sep := t.Muted.Render(strings.Repeat("│\n", innerHeight))

	// Combine panes side by side
	combined := lipgloss.JoinHorizontal(lipgloss.Top,
		treeView,
		sep,
		previewView,
	)
	rc.AddPart(combined)

	rc.Help = inj.help.View(inj)
	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

func (inj *Injection) renderTree(t *styles.Styles, width, height int) string {
	if len(inj.flatNodes) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).
			Render(t.Muted.Render("No files found"))
	}

	// Ensure scroll follows cursor
	visibleLines := height
	if inj.cursor >= inj.treeOff+visibleLines-2 {
		inj.treeOff = inj.cursor - visibleLines + 3
	}
	if inj.treeOff < 0 {
		inj.treeOff = 0
	}

	var lines []string
	end := min(inj.treeOff+visibleLines, len(inj.flatNodes))

	activePaneStyle := t.Base
	if inj.pane != injPaneTree {
		activePaneStyle = t.Muted
	}

	for idx := inj.treeOff; idx < end; idx++ {
		fn := inj.flatNodes[idx]
		node := fn.Node
		isCursor := idx == inj.cursor

		// Build line: [checkbox] [indent] [icon] name
		indent := snapshot.RenderTreeIndent(fn.Depth)

		var checkbox string
		if node.IsDir {
			if inj.anyChildSelected(node) {
				checkbox = t.Base.Foreground(t.Green).Render("[+]")
			} else {
				checkbox = t.Muted.Render("[ ]")
			}
		} else {
			if inj.scope.IsIgnored(node.Path) {
				checkbox = t.Base.Foreground(t.Red).Render("[x]")
			} else if inj.scope.IsSelected(node.Path) {
				checkbox = t.Base.Foreground(t.Green).Render("[*]")
			} else {
				checkbox = t.Muted.Render("[ ]")
			}
		}

		icon := fileTreeIcon(node.Name, node.IsDir, fn.Expanded)
		name := node.Name

		line := fmt.Sprintf("%s %s%s%s", checkbox, indent, icon, name)
		line = ansi.Truncate(line, width, "...")

		if isCursor {
			line = t.Base.Foreground(t.Primary).Bold(true).Render(line)
		} else if inj.scope.IsIgnored(node.Path) {
			line = t.Base.Foreground(t.Red).Render(line)
		} else {
			line = activePaneStyle.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad to fill height
	for len(lines) < visibleLines {
		lines = append(lines, "")
	}

	return lipgloss.NewStyle().Width(width).Height(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (inj *Injection) renderPreview(t *styles.Styles, width, height int) string {
	if inj.compiled == nil || inj.scope.SelectedCount() == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).
			Render(t.Muted.Render(" Select files to preview context"))
	}

	// Render compiled context
	preview := snapshot.FormatForInjection(inj.compiled)
	previewLines := strings.Split(preview, "\n")

	activePaneStyle := t.Base
	if inj.pane != injPanePreview {
		activePaneStyle = t.Muted
	}

	// Apply scroll offset
	if inj.previewOff >= len(previewLines) {
		inj.previewOff = max(0, len(previewLines)-1)
	}
	start := inj.previewOff
	end := min(start+height, len(previewLines))

	var lines []string
	for _, line := range previewLines[start:end] {
		truncated := ansi.Truncate(line, width-1, "...")
		// Highlight section headers
		if strings.HasPrefix(line, "===") {
			lines = append(lines, " "+t.Base.Foreground(t.Primary).Render(truncated))
		} else if strings.HasPrefix(line, "---") {
			lines = append(lines, " "+t.Base.Foreground(t.Secondary).Render(truncated))
		} else {
			lines = append(lines, " "+activePaneStyle.Render(truncated))
		}
	}

	// Pad to fill height
	for len(lines) < height {
		lines = append(lines, "")
	}

	return lipgloss.NewStyle().Width(width).Height(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// fileTreeIcon returns an icon for the tree entry.
func fileTreeIcon(name string, isDir, expanded bool) string {
	if isDir {
		if expanded {
			return "▾ "
		}
		return "▸ "
	}

	ext := snapshot.FileExt(name)
	switch ext {
	case "go":
		return "  "
	case "js", "jsx", "mjs":
		return "  "
	case "ts", "tsx":
		return "  "
	case "py":
		return "  "
	case "rs":
		return "  "
	case "md":
		return "  "
	case "json":
		return "  "
	case "yaml", "yml", "toml":
		return "  "
	case "html":
		return "  "
	case "css", "scss":
		return "  "
	case "sh", "bash", "zsh":
		return "  "
	default:
		return "  "
	}
}

func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ShortHelp implements help.KeyMap.
func (inj *Injection) ShortHelp() []key.Binding {
	return []key.Binding{
		inj.keyMap.Toggle,
		inj.keyMap.Expand,
		inj.keyMap.ToggleIgnore,
		inj.keyMap.SelectAll,
		inj.keyMap.SwitchPane,
		inj.keyMap.Apply,
		inj.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (inj *Injection) FullHelp() [][]key.Binding {
	return [][]key.Binding{inj.ShortHelp()}
}
