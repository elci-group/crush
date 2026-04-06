// Package mpv provides a client for controlling mpv via JSON IPC
// over a Unix socket. It manages the mpv subprocess lifecycle and
// exposes playback controls.
package mpv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the player playback state.
type State string

const (
	StateStopped State = "stopped"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateLoading State = "loading"
)

// NowPlaying holds metadata about the currently playing track.
type NowPlaying struct {
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Duration float64 `json:"duration"` // seconds
	Position float64 `json:"position"` // seconds
	Volume   int     `json:"volume"`   // 0-100
	State    State   `json:"state"`
	URL      string  `json:"url"`
}

// Progress returns playback progress as a fraction 0.0-1.0.
func (np NowPlaying) Progress() float64 {
	if np.Duration <= 0 {
		return 0
	}
	p := np.Position / np.Duration
	if p > 1 {
		return 1
	}
	return p
}

// FormatTime formats seconds as MM:SS.
func FormatTime(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	m := int(secs) / 60
	s := int(secs) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

// Player manages an mpv subprocess and communicates via JSON IPC.
type Player struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	socketPath string
	conn       net.Conn
	requestID  atomic.Int64
	nowPlaying NowPlaying
	cancel     context.CancelFunc

	// Queue of URLs to play.
	queue []QueueItem
}

// QueueItem represents a queued media item.
type QueueItem struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ipcCommand is the JSON structure for mpv IPC commands.
type ipcCommand struct {
	Command   []any `json:"command"`
	RequestID int64 `json:"request_id,omitempty"`
}

// ipcResponse is the JSON structure for mpv IPC responses.
type ipcResponse struct {
	Data      any    `json:"data"`
	RequestID int64  `json:"request_id"`
	Error     string `json:"error"`
}

// New creates a new Player. Call Start() to launch mpv.
func New() *Player {
	return &Player{
		nowPlaying: NowPlaying{State: StateStopped, Volume: 80},
	}
}

// Start launches the mpv subprocess with IPC enabled.
func (p *Player) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil {
		return nil // already running
	}

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	// Create socket path in temp dir
	p.socketPath = filepath.Join(os.TempDir(), fmt.Sprintf("crush-mpv-%d.sock", os.Getpid()))

	// Clean up stale socket
	os.Remove(p.socketPath)

	p.cmd = exec.CommandContext(ctx, "mpv",
		"--idle=yes",
		"--no-video",
		"--no-terminal",
		"--really-quiet",
		fmt.Sprintf("--input-ipc-server=%s", p.socketPath),
		fmt.Sprintf("--volume=%d", p.nowPlaying.Volume),
	)
	p.cmd.Stdout = nil
	p.cmd.Stderr = nil

	if err := p.cmd.Start(); err != nil {
		p.cmd = nil
		cancel()
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	// Wait for socket to become available
	go func() {
		defer cancel()
		if err := p.cmd.Wait(); err != nil {
			slog.Debug("mpv process exited", "error", err)
		}
		p.mu.Lock()
		p.cmd = nil
		p.conn = nil
		p.nowPlaying.State = StateStopped
		p.mu.Unlock()
	}()

	// Connect to IPC socket with retries
	for i := range 30 {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("unix", p.socketPath)
		if err == nil {
			p.conn = conn
			go p.pollStatus(ctx)
			slog.Info("Connected to mpv IPC", "socket", p.socketPath, "attempt", i+1)
			return nil
		}
	}

	return fmt.Errorf("timeout connecting to mpv IPC socket")
}

// Stop shuts down the mpv subprocess.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	os.Remove(p.socketPath)
	p.cmd = nil
	p.nowPlaying.State = StateStopped
}

// IsRunning returns true if mpv is running.
func (p *Player) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.conn != nil
}

// NowPlaying returns the current playback info.
func (p *Player) NowPlaying() NowPlaying {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nowPlaying
}

// Queue returns the current queue.
func (p *Player) Queue() []QueueItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]QueueItem, len(p.queue))
	copy(result, p.queue)
	return result
}

// LoadURL loads a URL (or yt-dlp compatible URL) into mpv.
func (p *Player) LoadURL(url, title string) error {
	p.mu.Lock()
	p.nowPlaying.State = StateLoading
	p.nowPlaying.Title = title
	p.nowPlaying.URL = url
	p.nowPlaying.Position = 0
	p.nowPlaying.Duration = 0
	p.mu.Unlock()

	return p.sendCommand("loadfile", url, "replace")
}

// Enqueue adds a URL to the play queue.
func (p *Player) Enqueue(url, title string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append(p.queue, QueueItem{URL: url, Title: title})
}

// PlayNext plays the next item from the queue.
func (p *Player) PlayNext() error {
	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		return nil
	}
	next := p.queue[0]
	p.queue = p.queue[1:]
	p.mu.Unlock()

	return p.LoadURL(next.URL, next.Title)
}

// TogglePause toggles play/pause.
func (p *Player) TogglePause() error {
	return p.sendCommand("cycle", "pause")
}

// Seek seeks by the given number of seconds (positive = forward).
func (p *Player) Seek(seconds float64) error {
	return p.sendCommand("seek", seconds, "relative")
}

// SetVolume sets volume (0-100).
func (p *Player) SetVolume(vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	p.mu.Lock()
	p.nowPlaying.Volume = vol
	p.mu.Unlock()
	return p.sendCommand("set_property", "volume", vol)
}

// sendCommand sends a JSON IPC command to mpv.
func (p *Player) sendCommand(args ...any) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected to mpv")
	}

	id := p.requestID.Add(1)
	cmd := ipcCommand{
		Command:   args,
		RequestID: id,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(data)
	return err
}

// getProperty fetches a property value from mpv.
func (p *Player) getProperty(name string) (any, error) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	id := p.requestID.Add(1)
	cmd := ipcCommand{
		Command:   []any{"get_property", name},
		RequestID: id,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	// Read response. We use a simple approach: read until newline.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	var resp ipcResponse
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" && resp.Error != "success" {
		return nil, fmt.Errorf("mpv error: %s", resp.Error)
	}
	return resp.Data, nil
}

// pollStatus periodically fetches playback state from mpv.
func (p *Player) pollStatus(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshStatus()
		}
	}
}

// refreshStatus fetches current state from mpv and updates NowPlaying.
func (p *Player) refreshStatus() {
	paused, err := p.getProperty("pause")
	if err != nil {
		return
	}

	pos, _ := p.getProperty("time-pos")
	dur, _ := p.getProperty("duration")
	title, _ := p.getProperty("media-title")
	vol, _ := p.getProperty("volume")

	p.mu.Lock()
	defer p.mu.Unlock()

	if b, ok := paused.(bool); ok {
		if b {
			p.nowPlaying.State = StatePaused
		} else {
			p.nowPlaying.State = StatePlaying
		}
	}

	if f, ok := pos.(float64); ok {
		p.nowPlaying.Position = f
	}
	if f, ok := dur.(float64); ok {
		p.nowPlaying.Duration = f
	}
	if s, ok := title.(string); ok && s != "" {
		p.nowPlaying.Title = s
	}
	if f, ok := vol.(float64); ok {
		p.nowPlaying.Volume = int(f)
	}
}
