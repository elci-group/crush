package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

const TTSID = "tts"

type ttsField int

const (
	ttsFieldEnabled ttsField = iota
	ttsFieldProvider
	ttsFieldAPIKey
	ttsFieldURL
	ttsFieldModel
	ttsFieldVoice
	ttsFieldPersonality
	ttsFieldSequester
	ttsFieldMax
)

var providers = []string{"elevenlabs", "openai", "groq"}

type TTS struct {
	com    *common.Common
	config *config.TTSConfig
	inputs []textinput.Model
	cursor ttsField
	help   help.Model
	keyMap struct {
		Up     key.Binding
		Down   key.Binding
		Toggle key.Binding
		Close  key.Binding
		Save   key.Binding
	}
}

func NewTTS(com *common.Common) *TTS {
	c := com.Config()
	ttsConf := c.TTS
	if ttsConf == nil {
		ttsConf = &config.TTSConfig{}
	}

	// create a deep copy to edit
	editConf := &config.TTSConfig{
		Enabled:         ttsConf.Enabled,
		Provider:        ttsConf.Provider,
		ElevenLabsKey:   ttsConf.ElevenLabsKey,
		ElevenLabsVoice: ttsConf.ElevenLabsVoice,
		GroqKey:         ttsConf.GroqKey,
		GeminiKey:       ttsConf.GeminiKey,
		OpenAIKey:       ttsConf.OpenAIKey,
		OpenAIURL:       ttsConf.OpenAIURL,
		OpenAIModel:     ttsConf.OpenAIModel,
		OpenAIVoice:     ttsConf.OpenAIVoice,
		Personality:     ttsConf.Personality,
		SequesterKeys:   ttsConf.SequesterKeys,
	}

	if editConf.Provider == "" {
		if editConf.ElevenLabsKey != "" {
			editConf.Provider = "elevenlabs"
		} else if editConf.OpenAIKey != "" {
			editConf.Provider = "openai"
		} else if editConf.GroqKey != "" {
			editConf.Provider = "groq"
		} else {
			editConf.Provider = "elevenlabs"
		}
	}

	m := &TTS{
		com:    com,
		config: editConf,
		inputs: make([]textinput.Model, ttsFieldMax),
	}

	t := com.Styles

	for i := range m.inputs {
		in := textinput.New()
		in.SetVirtualCursor(false)
		in.SetStyles(t.TextInput)
		m.inputs[i] = in
	}

	m.inputs[ttsFieldPersonality].Placeholder = "Personality (e.g. funny, snarky)"
	m.inputs[ttsFieldPersonality].SetValue(editConf.Personality)

	m.help = help.New()
	m.help.Styles = t.DialogHelpStyles()

	m.keyMap.Up = key.NewBinding(key.WithKeys("up", "shift+tab"), key.WithHelp("↑", "up"))
	m.keyMap.Down = key.NewBinding(key.WithKeys("down", "tab"), key.WithHelp("↓", "down"))
	m.keyMap.Toggle = key.NewBinding(key.WithKeys(" ", "enter", "right"), key.WithHelp("space", "toggle"))
	m.keyMap.Close = CloseKey
	m.keyMap.Save = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save"))

	m.loadProviderInputs()
	m.updateFocus()

	return m
}

func (t *TTS) loadProviderInputs() {
	switch t.config.Provider {
	case "elevenlabs":
		t.inputs[ttsFieldAPIKey].Placeholder = "ElevenLabs API Key"
		t.inputs[ttsFieldAPIKey].SetValue(t.config.ElevenLabsKey)
		t.inputs[ttsFieldVoice].Placeholder = "Voice ID (Default: 21m00Tcm4TlvDq8ikWAM)"
		t.inputs[ttsFieldVoice].SetValue(t.config.ElevenLabsVoice)
	case "openai":
		t.inputs[ttsFieldAPIKey].Placeholder = "OpenAI API Key"
		t.inputs[ttsFieldAPIKey].SetValue(t.config.OpenAIKey)
		t.inputs[ttsFieldURL].Placeholder = "API URL (Default: https://api.openai.com/v1/audio/speech)"
		t.inputs[ttsFieldURL].SetValue(t.config.OpenAIURL)
		t.inputs[ttsFieldModel].Placeholder = "Model (Default: tts-1)"
		t.inputs[ttsFieldModel].SetValue(t.config.OpenAIModel)
		t.inputs[ttsFieldVoice].Placeholder = "Voice (Default: alloy)"
		t.inputs[ttsFieldVoice].SetValue(t.config.OpenAIVoice)
	case "groq":
		t.inputs[ttsFieldAPIKey].Placeholder = "Groq API Key"
		t.inputs[ttsFieldAPIKey].SetValue(t.config.GroqKey)
	}
}

func (t *TTS) saveProviderInputs() {
	switch t.config.Provider {
	case "elevenlabs":
		t.config.ElevenLabsKey = t.inputs[ttsFieldAPIKey].Value()
		t.config.ElevenLabsVoice = t.inputs[ttsFieldVoice].Value()
	case "openai":
		t.config.OpenAIKey = t.inputs[ttsFieldAPIKey].Value()
		t.config.OpenAIURL = t.inputs[ttsFieldURL].Value()
		t.config.OpenAIModel = t.inputs[ttsFieldModel].Value()
		t.config.OpenAIVoice = t.inputs[ttsFieldVoice].Value()
	case "groq":
		t.config.GroqKey = t.inputs[ttsFieldAPIKey].Value()
	}
}

func (t *TTS) updateFocus() {
	for i := range t.inputs {
		if t.cursor == ttsField(i) {
			t.inputs[i].Focus()
		} else {
			t.inputs[i].Blur()
		}
	}
}

func (t *TTS) ID() string    { return TTSID }
func (t *TTS) Init() tea.Cmd { return textinput.Blink }

func (t *TTS) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, t.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, t.keyMap.Save):
			t.saveProviderInputs()
			t.config.Personality = t.inputs[ttsFieldPersonality].Value()

			t.com.Config().TTS = t.config
			// save config
			go func() {
				_ = t.com.Workspace.SetConfigField(config.ScopeGlobal, "tts", t.config)
			}()
			return ActionClose{}
		case key.Matches(msg, t.keyMap.Up):
			t.cursor--
			if t.cursor < 0 {
				t.cursor = ttsFieldMax - 1
			}
			t.updateFocus()
		case key.Matches(msg, t.keyMap.Down):
			t.cursor++
			if t.cursor >= ttsFieldMax {
				t.cursor = 0
			}
			t.updateFocus()
		case key.Matches(msg, t.keyMap.Toggle):
			if t.cursor == ttsFieldEnabled {
				t.config.Enabled = !t.config.Enabled
			} else if t.cursor == ttsFieldSequester {
				t.config.SequesterKeys = !t.config.SequesterKeys
			} else if t.cursor == ttsFieldProvider {
				t.saveProviderInputs()
				idx := 0
				for i, p := range providers {
					if p == t.config.Provider {
						idx = i
						break
					}
				}
				idx = (idx + 1) % len(providers)
				t.config.Provider = providers[idx]
				t.loadProviderInputs()
			}
		}
	}

	// update text inputs
	if t.cursor == ttsFieldAPIKey || t.cursor == ttsFieldURL || t.cursor == ttsFieldModel || t.cursor == ttsFieldVoice || t.cursor == ttsFieldPersonality {
		var cmd tea.Cmd
		t.inputs[t.cursor], cmd = t.inputs[t.cursor].Update(msg)
		if cmd != nil {
			return ActionCmd{cmd}
		}
	}

	return nil
}

func (t *TTS) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	sty := t.com.Styles
	rc := NewRenderContext(sty, 60)
	rc.Title = "TTS Settings"

	var lines []string

	renderToggle := func(idx ttsField, label string, value bool) string {
		cursor := "  "
		if t.cursor == idx {
			cursor = sty.Base.Foreground(sty.Primary).Render("> ")
		}
		valStr := "[ ]"
		if value {
			valStr = "[x]"
		}
		return cursor + label + " " + valStr
	}

	renderSelector := func(idx ttsField, label string, value string) string {
		cursor := "  "
		if t.cursor == idx {
			cursor = sty.Base.Foreground(sty.Primary).Render("> ")
		}
		return cursor + label + " " + sty.Base.Foreground(sty.Primary).Render("< "+value+" >")
	}

	renderInput := func(idx ttsField, label string) string {
		cursor := "  "
		if t.cursor == idx {
			cursor = sty.Base.Foreground(sty.Primary).Render("> ")
		}
		return cursor + label + "\n   " + t.inputs[idx].View()
	}

	lines = append(lines, renderToggle(ttsFieldEnabled, "Enable TTS", t.config.Enabled))
	lines = append(lines, "")
	lines = append(lines, renderSelector(ttsFieldProvider, "Provider:", t.config.Provider))
	lines = append(lines, "")
	lines = append(lines, renderInput(ttsFieldAPIKey, t.inputs[ttsFieldAPIKey].Placeholder+":"))

	if t.config.Provider == "openai" {
		lines = append(lines, renderInput(ttsFieldURL, "URL:"))
		lines = append(lines, renderInput(ttsFieldModel, "Model:"))
	}

	if t.config.Provider == "openai" || t.config.Provider == "elevenlabs" {
		lines = append(lines, renderInput(ttsFieldVoice, "Voice:"))
	}

	lines = append(lines, "")
	lines = append(lines, renderInput(ttsFieldPersonality, "Personality:"))
	lines = append(lines, "")
	lines = append(lines, renderToggle(ttsFieldSequester, "Sequester Keys", t.config.SequesterKeys))
	lines = append(lines, "")

	rc.AddPart(lipgloss.JoinVertical(lipgloss.Left, lines...))
	rc.Help = t.help.View(t)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

func (t *TTS) FullHelp() [][]key.Binding {
	return [][]key.Binding{t.ShortHelp()}
}

func (t *TTS) ShortHelp() []key.Binding {
	return []key.Binding{t.keyMap.Up, t.keyMap.Down, t.keyMap.Toggle, t.keyMap.Save, t.keyMap.Close}
}
