package dialog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/home"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// FileExplorerID is the identifier for the file explorer dialog.
const FileExplorerID = "fileexplorer"

const (
	fileExplorerMaxWidth  = 100
	fileExplorerMaxHeight = 30
	treeMinWidth          = 28
	previewMinWidth       = 30
	treePaneRatio         = 0.35
	maxPreviewBytes       = 128 * 1024 // 128KB preview limit
	maxPreviewLines       = 500
)

// Icons for the file tree.
const (
	iconDir      = "▸ "
	iconDirOpen  = "▾ "
	iconFile     = "  "
	iconSymlink  = "⤳ "
	iconGo       = "  "
	iconJS       = "  "
	iconTS       = "  "
	iconPython   = "  "
	iconRust     = "  "
	iconMarkdown = "  "
	iconJSON     = "  "
	iconYAML     = "  "
	iconHTML     = "  "
	iconCSS      = "  "
	iconShell    = "  "
	iconLock     = "  "
	iconGit      = "  "
	iconImage    = "  "
)

// fileEntry represents a single item in the file tree.
type fileEntry struct {
	name   string
	path   string
	isDir  bool
	isLink bool
	depth  int
	size   int64
}

// FileExplorer is a split-pane dialog with a file tree on the left
// and a bat/cat-style syntax-highlighted file preview on the right.
type FileExplorer struct {
	com *common.Common

	// Tree state
	cwd     string
	entries []fileEntry
	cursor  int
	offset  int // scroll offset for tree

	// Preview state
	previewContent string
	previewPath    string
	previewScroll  int
	previewLines   []string

	help   help.Model
	keyMap fileExplorerKeyMap
}

type fileExplorerKeyMap struct {
	Up            key.Binding
	Down          key.Binding
	Enter         key.Binding
	Back          key.Binding
	Navigate      key.Binding
	PreviewUp     key.Binding
	PreviewDown   key.Binding
	PreviewScroll key.Binding
	Top           key.Binding
	Bottom        key.Binding
	Select        key.Binding
	Close         key.Binding
}

var _ Dialog = (*FileExplorer)(nil)

// NewFileExplorer creates a new file explorer dialog.
func NewFileExplorer(com *common.Common) *FileExplorer {
	fe := &FileExplorer{
		com: com,
	}

	fe.help = help.New()
	fe.help.Styles = com.Styles.DialogHelpStyles()

	fe.keyMap = fileExplorerKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("right", "l", "enter"),
			key.WithHelp("→/enter", "open"),
		),
		Back: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "parent"),
		),
		Navigate: key.NewBinding(
			key.WithKeys("up", "down", "left", "right"),
			key.WithHelp("↑↓←→", "navigate"),
		),
		PreviewUp: key.NewBinding(
			key.WithKeys("shift+up", "K"),
		),
		PreviewDown: key.NewBinding(
			key.WithKeys("shift+down", "J"),
		),
		PreviewScroll: key.NewBinding(
			key.WithKeys("shift+up", "shift+down"),
			key.WithHelp("shift+↑↓", "scroll preview"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "bottom"),
		),
		Select: key.NewBinding(
			key.WithKeys("ctrl+y"),
			key.WithHelp("ctrl+y", "attach"),
		),
		Close: CloseKey,
	}

	// Initialize with workspace directory
	wd := com.Workspace.WorkingDir()
	if wd == "" {
		wd, _ = os.Getwd()
		if wd == "" {
			wd = home.Dir()
		}
	}
	fe.cwd = wd
	fe.loadDir()

	return fe
}

// ID implements Dialog.
func (fe *FileExplorer) ID() string {
	return FileExplorerID
}

// HandleMsg implements Dialog.
func (fe *FileExplorer) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, fe.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, fe.keyMap.Up):
			fe.moveCursor(-1)
			fe.updatePreview()
		case key.Matches(msg, fe.keyMap.Down):
			fe.moveCursor(1)
			fe.updatePreview()
		case key.Matches(msg, fe.keyMap.Top):
			fe.cursor = 0
			fe.offset = 0
			fe.updatePreview()
		case key.Matches(msg, fe.keyMap.Bottom):
			if len(fe.entries) > 0 {
				fe.cursor = len(fe.entries) - 1
			}
			fe.updatePreview()
		case key.Matches(msg, fe.keyMap.PreviewUp):
			if fe.previewScroll > 0 {
				fe.previewScroll--
			}
		case key.Matches(msg, fe.keyMap.PreviewDown):
			if fe.previewScroll < len(fe.previewLines)-1 {
				fe.previewScroll++
			}
		case key.Matches(msg, fe.keyMap.Enter):
			if fe.cursor < len(fe.entries) {
				entry := fe.entries[fe.cursor]
				if entry.isDir {
					fe.cwd = entry.path
					fe.cursor = 0
					fe.offset = 0
					fe.loadDir()
					fe.updatePreview()
				}
			}
		case key.Matches(msg, fe.keyMap.Back):
			parent := filepath.Dir(fe.cwd)
			if parent != fe.cwd {
				prev := filepath.Base(fe.cwd)
				fe.cwd = parent
				fe.cursor = 0
				fe.offset = 0
				fe.loadDir()
				// Try to re-select the directory we came from
				for i, e := range fe.entries {
					if e.name == prev {
						fe.cursor = i
						break
					}
				}
				fe.updatePreview()
			}
		case key.Matches(msg, fe.keyMap.Select):
			if fe.cursor < len(fe.entries) {
				entry := fe.entries[fe.cursor]
				if !entry.isDir {
					return ActionFilePickerSelected{Path: entry.path}
				}
			}
		}
	}
	return nil
}

// Draw implements Dialog.
func (fe *FileExplorer) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := fe.com.Styles
	dialogStyle := t.Dialog.View

	totalWidth := max(0, min(fileExplorerMaxWidth, area.Dx()-dialogStyle.GetHorizontalBorderSize()))
	totalHeight := max(0, min(fileExplorerMaxHeight, area.Dy()-dialogStyle.GetVerticalBorderSize()))
	innerWidth := totalWidth - dialogStyle.GetHorizontalFrameSize()
	innerHeight := totalHeight - dialogStyle.GetVerticalFrameSize()

	rc := NewRenderContext(t, totalWidth)
	rc.Title = "File Explorer"
	rc.TitleInfo = t.Muted.Render(" " + home.Short(fe.cwd))

	// Split inner width into tree pane and preview pane
	treeWidth := max(treeMinWidth, int(float64(innerWidth)*treePaneRatio))
	previewWidth := innerWidth - treeWidth - 1 // -1 for separator

	// Dynamically adjust based on preview content
	if len(fe.previewLines) > 0 {
		reqPreviewWidth := previewMinWidth
		for _, line := range fe.previewLines {
			w := ansi.StringWidth(line)
			if w > reqPreviewWidth {
				reqPreviewWidth = w
			}
		}

		lineNumWidth := len(fmt.Sprintf("%d", len(fe.previewLines)))
		reqPreviewWidth += lineNumWidth + 2 // +2 for " │" separator

		if reqPreviewWidth > previewWidth {
			treeWidth = max(treeMinWidth, innerWidth-reqPreviewWidth-1)
			previewWidth = innerWidth - treeWidth - 1
		} else {
			previewWidth = max(previewMinWidth, reqPreviewWidth)
			treeWidth = innerWidth - previewWidth - 1
		}
	}

	titleHeight := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight
	helpHeight := t.Dialog.HelpView.GetVerticalFrameSize() + 1
	contentHeight := max(1, innerHeight-titleHeight-helpHeight)

	// Render tree pane
	treeView := fe.renderTree(t, treeWidth, contentHeight)

	// Render preview pane
	previewView := fe.renderPreview(t, previewWidth, contentHeight)

	// Separator
	sep := fe.renderSeparator(t, contentHeight)

	// Join panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, treeView, sep, previewView)
	rc.AddPart(panes)

	rc.Help = fe.help.View(fe)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements help.KeyMap.
func (fe *FileExplorer) ShortHelp() []key.Binding {
	return []key.Binding{
		fe.keyMap.Navigate,
		fe.keyMap.PreviewScroll,
		fe.keyMap.Select,
		fe.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (fe *FileExplorer) FullHelp() [][]key.Binding {
	return [][]key.Binding{fe.ShortHelp()}
}

// loadDir reads the current working directory and populates entries.
func (fe *FileExplorer) loadDir() {
	fe.entries = nil
	fe.previewContent = ""
	fe.previewPath = ""
	fe.previewLines = nil
	fe.previewScroll = 0

	dirEntries, err := os.ReadDir(fe.cwd)
	if err != nil {
		return
	}

	var dirs, files []fileEntry
	for _, de := range dirEntries {
		name := de.Name()
		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(fe.cwd, name)
		info, err := de.Info()
		if err != nil {
			continue
		}

		entry := fileEntry{
			name:   name,
			path:   fullPath,
			isDir:  de.IsDir(),
			isLink: de.Type()&os.ModeSymlink != 0,
			size:   info.Size(),
		}
		if entry.isDir {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	fe.entries = append(dirs, files...)

	if len(fe.entries) > 0 {
		fe.updatePreview()
	}
}

// moveCursor moves the cursor by delta, clamping to valid range.
func (fe *FileExplorer) moveCursor(delta int) {
	if len(fe.entries) == 0 {
		return
	}
	fe.cursor += delta
	if fe.cursor < 0 {
		fe.cursor = 0
	}
	if fe.cursor >= len(fe.entries) {
		fe.cursor = len(fe.entries) - 1
	}
}

// updatePreview loads the file content for the currently selected entry.
func (fe *FileExplorer) updatePreview() {
	fe.previewScroll = 0
	fe.previewContent = ""
	fe.previewPath = ""
	fe.previewLines = nil

	if fe.cursor >= len(fe.entries) {
		return
	}
	entry := fe.entries[fe.cursor]
	if entry.isDir {
		// Show directory listing as preview
		children, err := os.ReadDir(entry.path)
		if err != nil {
			fe.previewContent = fmt.Sprintf("Cannot read directory: %s", err)
			fe.previewLines = []string{fe.previewContent}
			return
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("Directory: %d items", len(children)))
		lines = append(lines, "")
		for _, c := range children {
			prefix := "  "
			if c.IsDir() {
				prefix = iconDir
			}
			lines = append(lines, prefix+c.Name())
		}
		fe.previewContent = strings.Join(lines, "\n")
		fe.previewLines = lines
		fe.previewPath = ""
		return
	}

	// File preview
	if entry.size > maxPreviewBytes {
		fe.previewContent = fmt.Sprintf("File too large to preview (%s)", formatFileSize(entry.size))
		fe.previewLines = []string{fe.previewContent}
		return
	}

	if isBinaryExt(entry.name) {
		fe.previewContent = fmt.Sprintf("Binary file (%s)", formatFileSize(entry.size))
		fe.previewLines = []string{fe.previewContent}
		return
	}

	content, err := os.ReadFile(entry.path)
	if err != nil {
		fe.previewContent = fmt.Sprintf("Cannot read file: %s", err)
		fe.previewLines = []string{fe.previewContent}
		return
	}

	// Check if content looks binary
	if isBinaryContent(content) {
		fe.previewContent = fmt.Sprintf("Binary file (%s)", formatFileSize(entry.size))
		fe.previewLines = []string{fe.previewContent}
		return
	}

	text := string(content)
	lines := strings.Split(text, "\n")
	if len(lines) > maxPreviewLines {
		lines = lines[:maxPreviewLines]
		lines = append(lines, fmt.Sprintf("... (%d more lines)", len(strings.Split(text, "\n"))-maxPreviewLines))
	}

	fe.previewContent = strings.Join(lines, "\n")
	fe.previewLines = lines
	fe.previewPath = entry.path
}

// renderTree renders the file tree pane.
func (fe *FileExplorer) renderTree(t *styles.Styles, width, height int) string {
	if len(fe.entries) == 0 {
		empty := t.Muted.Render("(empty directory)")
		return lipgloss.NewStyle().Width(width).Height(height).Render(empty)
	}

	// Adjust scroll offset to keep cursor visible
	if fe.cursor < fe.offset {
		fe.offset = fe.cursor
	}
	if fe.cursor >= fe.offset+height {
		fe.offset = fe.cursor - height + 1
	}

	var lines []string
	end := min(fe.offset+height, len(fe.entries))
	for i := fe.offset; i < end; i++ {
		entry := fe.entries[i]
		icon := fileIcon(entry)
		name := entry.name
		if entry.isDir {
			name += "/"
		}

		line := icon + name
		lineWidth := width - 1 // leave room for cursor
		line = ansi.Truncate(line, lineWidth, "…")

		if i == fe.cursor {
			line = t.Dialog.SelectedItem.Width(width).Render(line)
		} else {
			if entry.isDir {
				line = t.Base.Foreground(t.Primary).Width(width).Render(line)
			} else {
				line = t.Base.Foreground(t.FgMuted).Width(width).Render(line)
			}
		}
		lines = append(lines, line)
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(""))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderPreview renders the bat/cat-style file preview pane with line numbers
// and syntax highlighting.
func (fe *FileExplorer) renderPreview(t *styles.Styles, width, height int) string {
	if fe.previewContent == "" {
		empty := t.Muted.Render("No preview available")
		return lipgloss.NewStyle().Width(width).Height(height).Render(empty)
	}

	var displayLines []string

	if fe.previewPath != "" {
		// Syntax-highlighted preview for files
		highlighted, err := common.SyntaxHighlight(t, fe.previewContent, fe.previewPath, t.BgBase)
		if err != nil {
			highlighted = fe.previewContent
		}
		displayLines = strings.Split(highlighted, "\n")
	} else {
		// Plain text for directory listings etc.
		displayLines = fe.previewLines
	}

	// Apply scroll offset
	if fe.previewScroll >= len(displayLines) {
		fe.previewScroll = max(0, len(displayLines)-1)
	}

	lineNumWidth := len(fmt.Sprintf("%d", min(fe.previewScroll+height, len(displayLines))))
	contentWidth := width - lineNumWidth - 2 // 2 for " │" separator

	var lines []string
	end := min(fe.previewScroll+height, len(displayLines))
	for i := fe.previewScroll; i < end; i++ {
		lineNum := t.LineNumber.Render(fmt.Sprintf("%*d", lineNumWidth, i+1))
		sep := t.Muted.Render("│")
		content := displayLines[i]
		// Truncate long lines
		content = ansi.Truncate(content, max(0, contentWidth), "…")

		lines = append(lines, fmt.Sprintf("%s%s%s", lineNum, sep, content))
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(""))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderSeparator renders a vertical separator between panes.
func (fe *FileExplorer) renderSeparator(t *styles.Styles, height int) string {
	sep := strings.Repeat("│\n", height-1) + "│"
	return t.Muted.Render(sep)
}

// fileIcon returns an icon string for the given file entry.
func fileIcon(e fileEntry) string {
	if e.isLink {
		return iconSymlink
	}
	if e.isDir {
		return iconDir
	}
	ext := strings.ToLower(filepath.Ext(e.name))
	switch ext {
	case ".go":
		return iconGo
	case ".js", ".jsx", ".mjs":
		return iconJS
	case ".ts", ".tsx":
		return iconTS
	case ".py":
		return iconPython
	case ".rs":
		return iconRust
	case ".md":
		return iconMarkdown
	case ".json":
		return iconJSON
	case ".yaml", ".yml":
		return iconYAML
	case ".html", ".htm":
		return iconHTML
	case ".css", ".scss", ".less":
		return iconCSS
	case ".sh", ".bash", ".zsh", ".fish":
		return iconShell
	case ".lock":
		return iconLock
	case ".gitignore", ".gitattributes", ".gitmodules":
		return iconGit
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp":
		return iconImage
	default:
		return iconFile
	}
}

// isBinaryExt returns true for known binary file extensions.
func isBinaryExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".bmp", ".tiff",
		".mp3", ".mp4", ".avi", ".mov", ".wav", ".flac", ".ogg",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj",
		".wasm", ".class", ".pyc", ".pyo",
		".db", ".sqlite", ".sqlite3",
		".pdf", ".doc", ".docx", ".xls", ".xlsx":
		return true
	}
	return false
}

// isBinaryContent heuristically detects binary content by checking for null bytes.
func isBinaryContent(data []byte) bool {
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}

// formatFileSize formats a file size in human-readable form.
func formatFileSize(size int64) string {
	switch {
	case size >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	case size >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
