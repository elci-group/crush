package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/charmbracelet/crush/internal/config"
)

func Speak(ctx context.Context, text string, cfg *config.TTSConfig) (string, error) {
	if !cfg.Enabled {
		return "", fmt.Errorf("TTS is disabled")
	}

	provider := cfg.Provider
	if provider == "" {
		if cfg.ElevenLabsKey != "" {
			provider = "elevenlabs"
		} else if cfg.GroqKey != "" {
			provider = "groq"
		} else if cfg.OpenAIKey != "" {
			provider = "openai"
		}
	}

	switch provider {
	case "elevenlabs":
		if cfg.ElevenLabsKey != "" {
			return speakElevenLabs(ctx, text, cfg)
		}
	case "groq":
		if cfg.GroqKey != "" {
			return speakOpenAI(ctx, text, cfg.GroqKey, "https://api.groq.com/openai/v1/audio/speech", "whisper-1", "alloy") // Groq does not have TTS yet actually
		}
	case "openai":
		if cfg.OpenAIKey != "" {
			url := cfg.OpenAIURL
			if url == "" {
				url = "https://api.openai.com/v1/audio/speech"
			}
			model := cfg.OpenAIModel
			if model == "" {
				model = "tts-1"
			}
			voice := cfg.OpenAIVoice
			if voice == "" {
				voice = "alloy"
			}
			return speakOpenAI(ctx, text, cfg.OpenAIKey, url, model, voice)
		}
	}

	return "", fmt.Errorf("no supported TTS provider configured or API key missing")
}

func speakElevenLabs(ctx context.Context, text string, cfg *config.TTSConfig) (string, error) {
	voiceID := cfg.ElevenLabsVoice
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Default voice ID
	}
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	modelID := "eleven_monolingual_v1"

	payload := map[string]interface{}{
		"text":     text,
		"model_id": modelID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", cfg.ElevenLabsKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("elevenlabs API error: %s (status %d)", string(b), resp.StatusCode)
	}

	return saveTempAudio(resp.Body, "mp3")
}

func speakOpenAI(ctx context.Context, text, apiKey, baseURL, model, voice string) (string, error) {
	payload := map[string]interface{}{
		"model": model,
		"input": text,
		"voice": voice,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI/Groq TTS API error: %s (status %d)", string(b), resp.StatusCode)
	}

	return saveTempAudio(resp.Body, "mp3")
}

func saveTempAudio(r io.Reader, ext string) (string, error) {
	tmpDir := os.TempDir()
	f, err := os.CreateTemp(tmpDir, "crush-tts-*."+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}

	return f.Name(), nil
}
