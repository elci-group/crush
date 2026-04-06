package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/agent/notify"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/fsext"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	headerDiag           = "╱"
	minHeaderDiags       = 3
	leftPadding          = 1
	rightPadding         = 1
	diagToDetailsSpacing = 1 // space between diagonal pattern and details section
)

type header struct {
	// cached logo and compact logo
	logo        string
	compactLogo string

	com     *common.Common
	width   int
	compact bool
}

// newHeader creates a new header model.
func newHeader(com *common.Common) *header {
	h := &header{
		com: com,
	}
	t := com.Styles
	h.compactLogo = t.Header.Charm.Render("Charm™") + " " +
		styles.ApplyBoldForegroundGrad(t, "CRUSH", t.Secondary, t.Primary) + " "
	return h
}

// drawHeader draws the header for the given session.
func (h *header) drawHeader(
	scr uv.Screen,
	area uv.Rectangle,
	session *session.Session,
	compact bool,
	detailsOpen bool,
	width int,
	completedBgSessions []notify.Notification,
	tabAnimFrame int,
) {
	t := h.com.Styles
	if width != h.width || compact != h.compact {
		h.logo = renderLogo(h.com.Styles, compact, width)
	}

	h.width = width
	h.compact = compact

	if !compact || session == nil {
		uv.NewStyledString(h.logo).Draw(scr, area)
		return
	}

	if session.ID == "" {
		return
	}

	var b strings.Builder
	b.WriteString(h.compactLogo)

	availDetailWidth := width - leftPadding - rightPadding - lipgloss.Width(b.String()) - minHeaderDiags - diagToDetailsSpacing
	lspErrorCount := 0
	for _, info := range h.com.Workspace.LSPGetStates() {
		lspErrorCount += info.DiagnosticCount
	}
	details := renderHeaderDetails(
		h.com,
		session,
		lspErrorCount,
		detailsOpen,
		availDetailWidth,
	)

	// Build the center title
	titleStr := session.Title
	if titleStr == "" {
		titleStr = "New Session"
	}
	leftArrow := "◀ "
	rightArrow := " ▶"

	var centerTitle string
	if len(completedBgSessions) > 0 {
		bgTitle := completedBgSessions[len(completedBgSessions)-1].SessionTitle
		if bgTitle == "" {
			bgTitle = "Task Complete"
		}
		maxOffset := 10
		offset := tabAnimFrame % maxOffset
		pad := strings.Repeat(" ", offset)
		centerTitle = fmt.Sprintf("%s%s %s   %s   %s %s%s", leftArrow, pad, bgTitle, titleStr, bgTitle, pad, rightArrow)
	} else {
		centerTitle = fmt.Sprintf("%s %s %s", leftArrow, titleStr, rightArrow)
	}
	centerTitle = t.Header.WorkingDir.Render(centerTitle)

	remainingWidth := width -
		lipgloss.Width(b.String()) -
		lipgloss.Width(details) -
		lipgloss.Width(centerTitle) -
		leftPadding -
		rightPadding -
		diagToDetailsSpacing

	if remainingWidth > 0 {
		leftPad := remainingWidth / 2
		rightPad := remainingWidth - leftPad
		b.WriteString(t.Header.Diagonals.Render(strings.Repeat(headerDiag, leftPad)))
		b.WriteString(centerTitle)
		b.WriteString(t.Header.Diagonals.Render(strings.Repeat(headerDiag, rightPad)))
		b.WriteString(" ")
	} else {
		b.WriteString(centerTitle)
		b.WriteString(" ")
	}

	b.WriteString(details)

	view := uv.NewStyledString(
		t.Base.Padding(0, rightPadding, 0, leftPadding).Render(ansi.Truncate(b.String(), width, "…")))
	view.Draw(scr, area)
}

// renderHeaderDetails renders the details section of the header.
func renderHeaderDetails(
	com *common.Common,
	session *session.Session,
	lspErrorCount int,
	detailsOpen bool,
	availWidth int,
) string {
	t := com.Styles

	var parts []string

	if lspErrorCount > 0 {
		parts = append(parts, t.LSP.ErrorDiagnostic.Render(fmt.Sprintf("%s%d", styles.LSPErrorIcon, lspErrorCount)))
	}

	agentCfg := com.Config().Agents[config.AgentCoder]
	model := com.Config().GetModelByType(agentCfg.Model)
	if model != nil && model.ContextWindow > 0 {
		percentage := (float64(session.CompletionTokens+session.PromptTokens) / float64(model.ContextWindow)) * 100
		formattedPercentage := t.Header.Percentage.Render(fmt.Sprintf("%d%%", int(percentage)))
		parts = append(parts, formattedPercentage)
	}

	const keystroke = "ctrl+d"
	if detailsOpen {
		parts = append(parts, t.Header.Keystroke.Render(keystroke)+t.Header.KeystrokeTip.Render(" close"))
	} else {
		parts = append(parts, t.Header.Keystroke.Render(keystroke)+t.Header.KeystrokeTip.Render(" open "))
	}

	dot := t.Header.Separator.Render(" • ")
	metadata := strings.Join(parts, dot)
	metadata = dot + metadata

	const dirTrimLimit = 4
	cwd := fsext.DirTrim(fsext.PrettyPath(com.Workspace.WorkingDir()), dirTrimLimit)
	cwd = t.Header.WorkingDir.Render(cwd)

	result := cwd + metadata
	return ansi.Truncate(result, max(0, availWidth), "…")
}
