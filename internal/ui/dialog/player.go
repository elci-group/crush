package dialog

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/mpv"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/util"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// PlayerID is the identifier for the player dialog.
const PlayerID = "player"

const (
	playerMaxWidth  = 65
	playerMaxHeight = 22
	progressBarChar = "━"
	progressDotChar = "●"
	progressDimChar = "─"
)

// playerMode represents which view the player dialog shows.
type playerMode int

const (
	playerModeControls playerMode = iota
	playerModeURL
	playerModeQueue
)

// ActionPlayerLoad is emitted when the user submits a URL to play.
type ActionPlayerLoad struct {
	URL   string
	Title string
}

// ActionPlayerEnqueue is emitted when the user enqueues a URL.
type ActionPlayerEnqueue struct {
	URL   string
	Title string
}

// Player is the mini player control dialog.
type Player struct {
	com    *common.Common
	player *mpv.Player

	mode  playerMode
	input textinput.Model
	help  help.Model

	keyMap playerKeyMap
}

type playerKeyMap struct {
	PlayPause   key.Binding
	SeekForward key.Binding
	SeekBack    key.Binding
	VolumeUp    key.Binding
	VolumeDown  key.Binding
	Next        key.Binding
	Open        key.Binding
	QueueView   key.Binding
	Submit      key.Binding
	Navigate    key.Binding
	Close       key.Binding
}

var _ Dialog = (*Player)(nil)

// NewPlayer creates a new Player dialog.
func NewPlayer(com *common.Common, p *mpv.Player) *Player {
	t := com.Styles
	pl := &Player{
		com:    com,
		player: p,
		mode:   playerModeControls,
	}

	h := help.New()
	h.Styles = t.DialogHelpStyles()
	pl.help = h

	pl.input = textinput.New()
	pl.input.SetVirtualCursor(false)
	pl.input.Placeholder = "Paste YouTube URL or media URL..."
	pl.input.SetStyles(t.TextInput)
	pl.input.Focus()

	pl.keyMap = playerKeyMap{
		PlayPause: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "play/pause"),
		),
		SeekForward: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "+10s"),
		),
		SeekBack: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "-10s"),
		),
		VolumeUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "vol+"),
		),
		VolumeDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "vol-"),
		),
		Next: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next"),
		),
		Open: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open URL"),
		),
		QueueView: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "queue"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Navigate: key.NewBinding(
			key.WithKeys("up", "down", "left", "right"),
			key.WithHelp("↑↓←→", "control"),
		),
		Close: CloseKey,
	}

	return pl
}

// ID implements Dialog.
func (pl *Player) ID() string {
	return PlayerID
}

// HandleMsg implements Dialog.
func (pl *Player) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch pl.mode {
		case playerModeURL:
			return pl.handleURLMode(msg)
		case playerModeQueue:
			return pl.handleQueueMode(msg)
		default:
			return pl.handleControlsMode(msg)
		}
	}
	return nil
}

func (pl *Player) handleControlsMode(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, pl.keyMap.Close):
		return ActionClose{}
	case key.Matches(msg, pl.keyMap.PlayPause):
		if err := pl.player.TogglePause(); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.SeekForward):
		if err := pl.player.Seek(10); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.SeekBack):
		if err := pl.player.Seek(-10); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.VolumeUp):
		np := pl.player.NowPlaying()
		if err := pl.player.SetVolume(np.Volume + 5); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.VolumeDown):
		np := pl.player.NowPlaying()
		if err := pl.player.SetVolume(np.Volume - 5); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.Next):
		if err := pl.player.PlayNext(); err != nil {
			return util.ReportError(err)
		}
	case key.Matches(msg, pl.keyMap.Open):
		pl.mode = playerModeURL
		pl.input.SetValue("")
		pl.input.Focus()
	case key.Matches(msg, pl.keyMap.QueueView):
		pl.mode = playerModeQueue
	}
	return nil
}

func (pl *Player) handleURLMode(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, pl.keyMap.Close):
		pl.mode = playerModeControls
		return nil
	case key.Matches(msg, pl.keyMap.Submit):
		url := strings.TrimSpace(pl.input.Value())
		if url == "" {
			pl.mode = playerModeControls
			return nil
		}
		pl.mode = playerModeControls

		// Resolve and play in background
		return ActionCmd{Cmd: func() tea.Msg {
			if mpv.IsYouTubeURL(url) && mpv.HasYtDlp() {
				meta, err := mpv.ResolveYouTube(context.Background(), url)
				if err != nil {
					return util.NewWarnMsg(fmt.Sprintf("yt-dlp: %s", err))
				}
				if err := pl.player.LoadURL(meta.URL, meta.Title); err != nil {
					return util.NewWarnMsg(fmt.Sprintf("mpv: %s", err))
				}
				return util.NewInfoMsg(fmt.Sprintf("Playing: %s", meta.Title))
			}
			if err := pl.player.LoadURL(url, url); err != nil {
				return util.NewWarnMsg(fmt.Sprintf("mpv: %s", err))
			}
			return util.NewInfoMsg("Playing URL")
		}}
	default:
		var cmd tea.Cmd
		pl.input, cmd = pl.input.Update(msg)
		if cmd != nil {
			return ActionCmd{Cmd: cmd}
		}
	}
	return nil
}

func (pl *Player) handleQueueMode(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, pl.keyMap.Close):
		pl.mode = playerModeControls
	}
	return nil
}

// Cursor returns the cursor for the URL input mode.
func (pl *Player) Cursor() *tea.Cursor {
	if pl.mode == playerModeURL {
		return InputCursor(pl.com.Styles, pl.input.Cursor())
	}
	return nil
}

// Draw implements Dialog.
func (pl *Player) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := pl.com.Styles
	dialogStyle := t.Dialog.View

	width := max(0, min(playerMaxWidth, area.Dx()-dialogStyle.GetHorizontalBorderSize()))
	innerWidth := width - dialogStyle.GetHorizontalFrameSize()

	rc := NewRenderContext(t, width)

	switch pl.mode {
	case playerModeURL:
		rc.Title = "Open URL"
		inputView := t.Dialog.InputPrompt.Render(pl.input.View())
		rc.AddPart(inputView)
		rc.Help = pl.help.View(pl)
		view := rc.Render()
		cur := pl.Cursor()
		DrawCenterCursor(scr, area, view, cur)
		return cur

	case playerModeQueue:
		rc.Title = "Queue"
		queue := pl.player.Queue()
		if len(queue) == 0 {
			rc.AddPart(t.Muted.Render("Queue is empty"))
		} else {
			var lines []string
			for i, item := range queue {
				title := item.Title
				if title == "" {
					title = item.URL
				}
				line := fmt.Sprintf("%d. %s", i+1, title)
				line = ansi.Truncate(line, innerWidth, "…")
				lines = append(lines, t.Base.Render(line))
			}
			rc.AddPart(lipgloss.JoinVertical(lipgloss.Left, lines...))
		}
		rc.Help = pl.help.View(pl)
		view := rc.Render()
		DrawCenter(scr, area, view)
		return nil

	default:
		rc.Title = "Player"
		np := pl.player.NowPlaying()

		// Now playing info
		rc.AddPart(pl.renderNowPlaying(t, np, innerWidth))

		// Progress bar
		rc.AddPart(pl.renderProgress(t, np, innerWidth))

		// Volume bar
		rc.AddPart(pl.renderVolume(t, np, innerWidth))

		// Queue summary
		queue := pl.player.Queue()
		if len(queue) > 0 {
			queueInfo := t.Muted.Render(fmt.Sprintf("%d items in queue", len(queue)))
			rc.AddPart(queueInfo)
		}

		rc.Help = pl.help.View(pl)
		view := rc.Render()
		DrawCenter(scr, area, view)
		return nil
	}
}

func (pl *Player) renderNowPlaying(t *styles.Styles, np mpv.NowPlaying, width int) string {
	var stateIcon string
	switch np.State {
	case mpv.StatePlaying:
		stateIcon = t.Base.Foreground(t.Green).Render("▶")
	case mpv.StatePaused:
		stateIcon = t.Base.Foreground(t.Yellow).Render("⏸")
	case mpv.StateLoading:
		stateIcon = t.Base.Foreground(t.Blue).Render("⟳")
	default:
		stateIcon = t.Muted.Render("⏹")
	}

	title := np.Title
	if title == "" {
		title = "No track loaded"
	}
	title = ansi.Truncate(title, width-4, "…")

	if np.Artist != "" {
		artist := ansi.Truncate(np.Artist, width-4, "…")
		return fmt.Sprintf("%s %s\n  %s", stateIcon, t.Base.Render(title), t.Muted.Render(artist))
	}
	return fmt.Sprintf("%s %s", stateIcon, t.Base.Render(title))
}

func (pl *Player) renderProgress(t *styles.Styles, np mpv.NowPlaying, width int) string {
	posStr := mpv.FormatTime(np.Position)
	durStr := mpv.FormatTime(np.Duration)
	timeStr := fmt.Sprintf("%s / %s", posStr, durStr)
	timeWidth := len(timeStr) + 2

	barWidth := width - timeWidth - 1
	if barWidth < 5 {
		return t.Muted.Render(timeStr)
	}

	filled := int(np.Progress() * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	bar := t.Base.Foreground(t.Primary).Render(strings.Repeat(progressBarChar, filled))
	if filled < barWidth {
		bar += t.Base.Foreground(t.Primary).Render(progressDotChar)
		if empty > 1 {
			bar += t.Muted.Render(strings.Repeat(progressDimChar, empty-1))
		}
	}

	return fmt.Sprintf("%s %s", bar, t.Subtle.Render(timeStr))
}

func (pl *Player) renderVolume(t *styles.Styles, np mpv.NowPlaying, width int) string {
	label := "Vol"
	volStr := fmt.Sprintf("%d%%", np.Volume)
	barWidth := width - len(label) - len(volStr) - 4
	if barWidth < 5 {
		return t.Muted.Render(fmt.Sprintf("%s: %s", label, volStr))
	}

	filled := int(float64(np.Volume) / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	bar := t.Base.Foreground(t.Secondary).Render(strings.Repeat(progressBarChar, filled))
	bar += t.Muted.Render(strings.Repeat(progressDimChar, empty))

	return fmt.Sprintf("%s %s %s", t.Subtle.Render(label), bar, t.Subtle.Render(volStr))
}

// ShortHelp implements help.KeyMap.
func (pl *Player) ShortHelp() []key.Binding {
	switch pl.mode {
	case playerModeURL:
		return []key.Binding{pl.keyMap.Submit, pl.keyMap.Close}
	case playerModeQueue:
		return []key.Binding{pl.keyMap.Close}
	default:
		return []key.Binding{
			pl.keyMap.PlayPause,
			pl.keyMap.Navigate,
			pl.keyMap.Open,
			pl.keyMap.Next,
			pl.keyMap.QueueView,
			pl.keyMap.Close,
		}
	}
}

// FullHelp implements help.KeyMap.
func (pl *Player) FullHelp() [][]key.Binding {
	return [][]key.Binding{pl.ShortHelp()}
}

// NowPlayingSidebar renders a compact now-playing widget for the sidebar.
func NowPlayingSidebar(t *styles.Styles, p *mpv.Player, width int) string {
	if p == nil || !p.IsRunning() {
		return ""
	}

	np := p.NowPlaying()
	if np.State == mpv.StateStopped {
		return ""
	}

	title := common.Section(t, t.ResourceGroupTitle.Render("Now Playing"), width)

	var stateIcon string
	switch np.State {
	case mpv.StatePlaying:
		stateIcon = t.ResourceOnlineIcon.String()
	case mpv.StatePaused:
		stateIcon = t.ResourceBusyIcon.String()
	case mpv.StateLoading:
		stateIcon = t.ResourceBusyIcon.String()
	default:
		stateIcon = t.ResourceOfflineIcon.String()
	}

	trackTitle := np.Title
	if trackTitle == "" {
		trackTitle = "Loading..."
	}

	trackLine := common.Status(t, common.StatusOpts{
		Icon:  stateIcon,
		Title: trackTitle,
	}, width)

	// Mini progress bar
	barWidth := width - 2
	if barWidth < 3 {
		return lipgloss.NewStyle().Width(width).Render(
			fmt.Sprintf("%s\n\n%s", title, trackLine))
	}

	posStr := mpv.FormatTime(np.Position)
	durStr := mpv.FormatTime(np.Duration)
	timeStr := t.Subtle.Render(fmt.Sprintf(" %s/%s", posStr, durStr))
	timeWidth := lipgloss.Width(timeStr)

	progWidth := barWidth - timeWidth
	if progWidth < 3 {
		progWidth = barWidth
		timeStr = ""
	}

	filled := int(np.Progress() * float64(progWidth))
	if filled > progWidth {
		filled = progWidth
	}
	empty := progWidth - filled

	bar := t.Base.Foreground(t.Primary).Render(strings.Repeat("━", filled))
	bar += t.Muted.Render(strings.Repeat("─", empty))
	progressLine := " " + bar + timeStr

	return lipgloss.NewStyle().Width(width).Render(
		fmt.Sprintf("%s\n\n%s\n%s", title, trackLine, progressLine))
}
