package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/config"
)

type Recorder struct {
	cmd      *exec.Cmd
	FilePath string
}

func StartRecording() (*Recorder, error) {
	tmpDir := os.TempDir()
	f, err := os.CreateTemp(tmpDir, "crush-stt-*.wav")
	if err != nil {
		return nil, err
	}
	f.Close()

	cmd := exec.Command("rec", "-q", "-c", "1", "-r", "16000", f.Name())
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start recording (is sox/rec installed?): %w", err)
	}

	return &Recorder{
		cmd:      cmd,
		FilePath: f.Name(),
	}, nil
}

func (r *Recorder) Stop() error {
	if r.cmd != nil && r.cmd.Process != nil {
		return r.cmd.Process.Kill()
	}
	return nil
}

func Transcribe(ctx context.Context, audioPath string, cfg *config.STTConfig) (string, error) {
	if !cfg.Enabled {
		return "", fmt.Errorf("STT is disabled")
	}

	provider := cfg.Provider
	if provider == "" {
		if cfg.FasterWhisper != "" {
			provider = "local"
		} else if cfg.GroqKey != "" {
			provider = "groq"
		} else if cfg.OpenAIKey != "" {
			provider = "openai"
		}
	}

	switch provider {
	case "local":
		return transcribeLocal(ctx, audioPath, cfg.FasterWhisper)
	case "groq":
		return transcribeOpenAI(ctx, audioPath, cfg.GroqKey, "https://api.groq.com/openai/v1/audio/transcriptions", "whisper-large-v3-turbo")
	case "openai":
		url := cfg.OpenAIURL
		if url == "" {
			url = "https://api.openai.com/v1/audio/transcriptions"
		}
		model := cfg.OpenAIModel
		if model == "" {
			model = "whisper-1"
		}
		return transcribeOpenAI(ctx, audioPath, cfg.OpenAIKey, url, model)
	}

	return "", fmt.Errorf("no supported STT provider configured")
}

func transcribeLocal(ctx context.Context, audioPath string, fasterWhisperPath string) (string, error) {
	// Execute a local faster-whisper cli if available.
	// We assume a simple CLI wrapper that takes --audio file and outputs text to stdout.
	if fasterWhisperPath == "" {
		fasterWhisperPath = "faster-whisper-cli" // default assumption
	}
	cmd := exec.CommandContext(ctx, fasterWhisperPath, audioPath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("local transcription failed: %w", err)
	}
	return string(bytes.TrimSpace(out)), nil
}

func transcribeOpenAI(ctx context.Context, audioPath string, apiKey, baseURL, model string) (string, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	f, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fw, err := w.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(fw, f); err != nil {
		return "", err
	}
	
	if err := w.WriteField("model", model); err != nil {
		return "", err
	}

	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, &b)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("STT API error: %s (status %d)", string(respBody), resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.Text, nil
}
