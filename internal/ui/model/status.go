package model

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/util"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// DefaultStatusTTL is the default time-to-live for status messages.
const DefaultStatusTTL = 5 * time.Second

type statusTickMsg struct{}

func StatusTickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*150, func(time.Time) tea.Msg {
		return statusTickMsg{}
	})
}

// Status is the status bar and help model.
type Status struct {
	com          *common.Common
	hideHelp     bool
	helpKm       help.KeyMap
	msg          util.InfoMsg
	tickerOffset int
	width        int
}

// NewStatus creates a new status bar and help model.
func NewStatus(com *common.Common, km help.KeyMap) *Status {
	s := new(Status)
	s.com = com
	s.helpKm = km
	return s
}

func (s *Status) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case statusTickMsg:
		s.tickerOffset++
		return StatusTickCmd()
	}
	return nil
}

// SetInfoMsg sets the status info message.
func (s *Status) SetInfoMsg(msg util.InfoMsg) {
	s.msg = msg
}

// ClearInfoMsg clears the status info message.
func (s *Status) ClearInfoMsg() {
	s.msg = util.InfoMsg{}
}

// SetWidth sets the width of the status bar and help view.
func (s *Status) SetWidth(width int) {
	s.width = width
}

// ShowingAll returns whether the full help view is shown.
func (s *Status) ShowingAll() bool {
	return false
}

// ToggleHelp toggles the full help view.
func (s *Status) ToggleHelp() {
	// No-op for scrolling ticker
}

// SetHideHelp sets whether the app is on the onboarding flow.
func (s *Status) SetHideHelp(hideHelp bool) {
	s.hideHelp = hideHelp
}

var descEmojis = map[string]string{
	"quit":          "🛑",
	"more":          "➕",
	"commands":      "🛠️",
	"models":        "🧠",
	"suspend":       "💤",
	"sessions":      "📂",
	"tabs menu":     "📑",
	"prev tab":      "⏪",
	"next tab":      "⏩",
	"file explorer": "📁",
	"tts settings":  "🔊",
	"change focus":  "🔁",
	"add file":      "📄",
	"send":          "🚀",
	"open editor":   "📝",
	"newline":       "⏎",
	"add image":     "🖼️",
	"cancel":        "🚫",
}

// Draw draws the status bar onto the screen.
func (s *Status) Draw(scr uv.Screen, area uv.Rectangle) {
	if !s.hideHelp {
		// Build the ticker string
		var parts []string
		binds := s.helpKm.ShortHelp()

		keyStyle := s.com.Styles.Base.Foreground(s.com.Styles.Primary).Bold(true)
		descStyle := s.com.Styles.Base.Foreground(s.com.Styles.Secondary)
		sepStyle := s.com.Styles.Muted

		for _, b := range binds {
			if !b.Enabled() {
				continue
			}
			keys := b.Keys()
			if len(keys) == 0 {
				continue
			}
			desc := b.Help().Desc

			// Replace keys with symbols if possible
			keyStr := strings.Join(keys, "/")
			keyStr = strings.ReplaceAll(keyStr, "ctrl+", "⌃")
			keyStr = strings.ReplaceAll(keyStr, "shift+", "⇧")
			keyStr = strings.ReplaceAll(keyStr, "alt+", "⌥")
			keyStr = strings.ReplaceAll(keyStr, "enter", "↵")
			keyStr = strings.ReplaceAll(keyStr, "tab", "⇥")
			keyStr = strings.ReplaceAll(keyStr, "esc", "⎋")
			keyStr = strings.ReplaceAll(keyStr, "up", "↑")
			keyStr = strings.ReplaceAll(keyStr, "down", "↓")
			keyStr = strings.ReplaceAll(keyStr, "left", "←")
			keyStr = strings.ReplaceAll(keyStr, "right", "→")

			emoji := descEmojis[strings.ToLower(desc)]
			if emoji == "" {
				emoji = "✨"
			}

			part := keyStyle.Render(keyStr) + " " + descStyle.Render(desc) + " " + emoji
			parts = append(parts, part)
		}

		fullStr := strings.Join(parts, sepStyle.Render(" • "))

		// Render scrolling ticker
		visibleWidth := area.Dx() - s.com.Styles.Status.Help.GetPaddingLeft() - s.com.Styles.Status.Help.GetPaddingRight()
		if visibleWidth > 0 && len(fullStr) > 0 {
			cleanStr := ansi.Strip(fullStr)
			strLen := ansi.StringWidth(cleanStr)

			separator := sepStyle.Render(" | ")
			// We append the string to itself with a separator to allow continuous scrolling
			scrollingStr := fullStr + separator + fullStr + separator + fullStr

			offset := s.tickerOffset % (strLen + 3) // +3 for the " | " separator

			// Truncate from left up to offset
			truncLeft := ansi.TruncateLeft(scrollingStr, offset, "")

			// Then truncate from right up to visible width
			finalStr := ansi.Truncate(truncLeft, visibleWidth, "")

			finalStr = s.com.Styles.Status.Help.Render(finalStr)
			uv.NewStyledString(finalStr).Draw(scr, area)
		}
	}

	// Render notifications
	if s.msg.IsEmpty() {
		return
	}

	var indStyle lipgloss.Style
	var msgStyle lipgloss.Style
	switch s.msg.Type {
	case util.InfoTypeError:
		indStyle = s.com.Styles.Status.ErrorIndicator
		msgStyle = s.com.Styles.Status.ErrorMessage
	case util.InfoTypeWarn:
		indStyle = s.com.Styles.Status.WarnIndicator
		msgStyle = s.com.Styles.Status.WarnMessage
	case util.InfoTypeUpdate:
		indStyle = s.com.Styles.Status.UpdateIndicator
		msgStyle = s.com.Styles.Status.UpdateMessage
	case util.InfoTypeInfo:
		indStyle = s.com.Styles.Status.InfoIndicator
		msgStyle = s.com.Styles.Status.InfoMessage
	case util.InfoTypeSuccess:
		indStyle = s.com.Styles.Status.SuccessIndicator
		msgStyle = s.com.Styles.Status.SuccessMessage
	}

	ind := indStyle.String()
	indWidth := lipgloss.Width(ind)
	msg := strings.Join(strings.Split(s.msg.Msg, "\n"), " ")
	msgWidth := lipgloss.Width(msg)
	msg = ansi.Truncate(msg, area.Dx()-indWidth-msgWidth, "…")
	padWidth := max(0, area.Dx()-indWidth-msgWidth)
	msg += strings.Repeat(" ", padWidth)
	info := msgStyle.Render(msg)

	// Draw the info message over the help view
	uv.NewStyledString(ind+info).Draw(scr, area)
}

// clearInfoMsgCmd returns a command that clears the info message after the
// given TTL.
func clearInfoMsgCmd(ttl time.Duration) tea.Cmd {
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return util.ClearStatusMsg{}
	})
}
