package dialog

import (
	"fmt"
	"image/color"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

const SplitChatID = "split_chat"

type Pane struct {
	ID    int
	Color color.Color
	Model string
}

type SplitChat struct {
	com       *common.Common
	panes     []Pane
	activeIdx int
	width     int
	height    int
	palette   []color.Color
}

func NewSplitChat(com *common.Common) *SplitChat {
	t := com.Styles
	return &SplitChat{
		com:       com,
		panes:     []Pane{{ID: 1, Color: t.Primary, Model: "Global Default"}},
		activeIdx: 0,
		palette: []color.Color{
			t.Primary, t.Secondary, t.Green, t.Blue, t.Yellow, t.Red, t.Tertiary, t.White,
		},
	}
}

func (s *SplitChat) ID() string {
	return SplitChatID
}

func (s *SplitChat) Init() tea.Cmd {
	return nil
}

func (s *SplitChat) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case msg.String() == "esc" || msg.String() == "ctrl+c":
			return ActionClose{}
		case msg.String() == "ctrl+s":
			if len(s.panes) < 8 {
				newID := len(s.panes) + 1
				color := s.palette[newID%len(s.palette)]
				s.panes = append(s.panes, Pane{ID: newID, Color: color, Model: "Local Model"})
				s.activeIdx = len(s.panes) - 1
			}
		case msg.String() == "ctrl+[" || msg.String() == "alt+[":
			if len(s.panes) > 0 {
				s.activeIdx = (s.activeIdx - 1 + len(s.panes)) % len(s.panes)
			}
		case msg.String() == "ctrl+]" || msg.String() == "alt+]":
			if len(s.panes) > 0 {
				s.activeIdx = (s.activeIdx + 1) % len(s.panes)
			}
		case msg.String() == "ctrl+<" || msg.String() == "alt+<":
			if len(s.panes) > 1 {
				s.panes = append(s.panes[:s.activeIdx], s.panes[s.activeIdx+1:]...)
				if s.activeIdx >= len(s.panes) {
					s.activeIdx = len(s.panes) - 1
				}
			} else {
				return ActionClose{}
			}
		case msg.String() == "ctrl+>" || msg.String() == "alt+>":
			// Suspend chat - for now, just cycle
			if len(s.panes) > 0 {
				s.activeIdx = (s.activeIdx + 1) % len(s.panes)
			}
		case msg.String() == "m" || msg.String() == "c":
			// Mock changing model/color
			if len(s.panes) > 0 {
				s.panes[s.activeIdx].Model = "GPT-4"
			}
		}
	}
	return nil
}

func (s *SplitChat) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	s.width = area.Dx()
	s.height = area.Dy()

	if len(s.panes) == 0 {
		return nil
	}

	paneWidth := s.width / len(s.panes)

	for i, pane := range s.panes {
		paneRect := area
		paneRect.Min.X = area.Min.X + (i * paneWidth)
		paneRect.Max.X = area.Min.X + ((i + 1) * paneWidth)
		if i == len(s.panes)-1 {
			paneRect.Max.X = area.Max.X
		}

		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
		if i == s.activeIdx {
			style = style.BorderForeground(pane.Color)
		} else {
			style = style.BorderForeground(s.com.Styles.Border)
		}

		content := fmt.Sprintf("Chat %d\nModel: %s\nColor: %v", pane.ID, pane.Model, pane.Color)
		view := style.Width(paneRect.Dx() - 2).Height(paneRect.Dy() - 2).Render(content)
		uv.NewStyledString(view).Draw(scr, paneRect)
	}

	return nil
}

func (s *SplitChat) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "split")),
		key.NewBinding(key.WithKeys("ctrl+[", "ctrl+]"), key.WithHelp("ctrl+[/]", "cycle")),
		key.NewBinding(key.WithKeys("ctrl+<"), key.WithHelp("ctrl+<", "terminate")),
		key.NewBinding(key.WithKeys("ctrl+>"), key.WithHelp("ctrl+>", "suspend")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
}
