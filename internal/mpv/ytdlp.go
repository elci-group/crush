package mpv

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// VideoMeta holds metadata extracted by yt-dlp.
type VideoMeta struct {
	Title    string  `json:"title"`
	Uploader string  `json:"uploader"`
	Duration float64 `json:"duration"`
	URL      string  `json:"url"` // direct stream URL
}

// ResolveYouTube uses yt-dlp to extract audio stream URL and metadata
// from a YouTube URL. Returns the best audio-only stream.
func ResolveYouTube(ctx context.Context, youtubeURL string) (*VideoMeta, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--no-download",
		"--print-json",
		"-f", "bestaudio",
		"--no-playlist",
		youtubeURL,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp failed: %w", err)
	}

	var info struct {
		Title    string  `json:"title"`
		Uploader string  `json:"uploader"`
		Duration float64 `json:"duration"`
		URL      string  `json:"url"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp output: %w", err)
	}

	return &VideoMeta{
		Title:    info.Title,
		Uploader: info.Uploader,
		Duration: info.Duration,
		URL:      info.URL,
	}, nil
}

// IsYouTubeURL returns true if the URL looks like a YouTube URL.
func IsYouTubeURL(url string) bool {
	url = strings.ToLower(url)
	return strings.Contains(url, "youtube.com/watch") ||
		strings.Contains(url, "youtu.be/") ||
		strings.Contains(url, "youtube.com/shorts/") ||
		strings.Contains(url, "music.youtube.com/")
}

// HasYtDlp returns true if yt-dlp is available on the system.
func HasYtDlp() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

// HasMpv returns true if mpv is available on the system.
func HasMpv() bool {
	_, err := exec.LookPath("mpv")
	return err == nil
}
