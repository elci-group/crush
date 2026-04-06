// Package kaptaind provides daemon lifecycle management, health monitoring,
// and service registration for the Kaptaind backend integration.
package kaptaind

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/version"
)

// State represents the current daemon state.
type State string

const (
	StateStarting     State = "starting"
	StateRunning      State = "running"
	StateDegraded     State = "degraded"
	StateShuttingDown State = "shutting_down"
	StateStopped      State = "stopped"
)

// HealthStatus represents the daemon health check result.
type HealthStatus struct {
	State      State     `json:"state"`
	Uptime     string    `json:"uptime"`
	UptimeSecs int64     `json:"uptime_secs"`
	PID        int       `json:"pid"`
	GoRoutines int       `json:"goroutines"`
	MemAllocMB float64   `json:"mem_alloc_mb"`
	NumGC      uint32    `json:"num_gc"`
	Workspaces int       `json:"workspaces"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Registration holds daemon registration metadata for service discovery.
type Registration struct {
	DaemonID  string    `json:"daemon_id"`
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	Platform  string    `json:"platform"`
	StartedAt time.Time `json:"started_at"`
	Host      string    `json:"host"`
	PID       int       `json:"pid"`
	Channel   string    `json:"channel"`
}

// WorkspaceCountFunc returns the current number of active workspaces.
type WorkspaceCountFunc func() int

// Daemon manages the Kaptaind daemon lifecycle and health monitoring.
type Daemon struct {
	mu             sync.RWMutex
	state          State
	startedAt      time.Time
	daemonID       string
	host           string
	workspaceCount WorkspaceCountFunc

	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Daemon instance and starts health monitoring.
func New(ctx context.Context, daemonID, host string, wsCount WorkspaceCountFunc) *Daemon {
	ctx, cancel := context.WithCancel(ctx)
	hostname, _ := os.Hostname()

	d := &Daemon{
		state:          StateStarting,
		startedAt:      time.Now(),
		daemonID:       daemonID,
		host:           host,
		workspaceCount: wsCount,
		ctx:            ctx,
		cancel:         cancel,
	}

	if hostname != "" {
		d.host = hostname
	}

	go d.monitor()
	return d
}

// MarkRunning transitions the daemon to running state.
func (d *Daemon) MarkRunning() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = StateRunning
	slog.Info("Kaptaind daemon running", "id", d.daemonID, "pid", os.Getpid())
}

// MarkDegraded transitions the daemon to degraded state with a reason.
func (d *Daemon) MarkDegraded(reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = StateDegraded
	slog.Warn("Kaptaind daemon degraded", "id", d.daemonID, "reason", reason)
}

// Shutdown gracefully shuts down the daemon.
func (d *Daemon) Shutdown() {
	d.mu.Lock()
	d.state = StateShuttingDown
	d.mu.Unlock()
	d.cancel()
	d.mu.Lock()
	d.state = StateStopped
	d.mu.Unlock()
	slog.Info("Kaptaind daemon stopped", "id", d.daemonID)
}

// Health returns the current health status.
func (d *Daemon) Health() HealthStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(d.startedAt)
	wsCount := 0
	if d.workspaceCount != nil {
		wsCount = d.workspaceCount()
	}

	return HealthStatus{
		State:      d.state,
		Uptime:     formatUptime(uptime),
		UptimeSecs: int64(uptime.Seconds()),
		PID:        os.Getpid(),
		GoRoutines: runtime.NumGoroutine(),
		MemAllocMB: float64(memStats.Alloc) / 1024 / 1024,
		NumGC:      memStats.NumGC,
		Workspaces: wsCount,
		CheckedAt:  time.Now(),
	}
}

// Registration returns the daemon's registration info.
func (d *Daemon) Registration() Registration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return Registration{
		DaemonID:  d.daemonID,
		Version:   version.Version,
		Commit:    version.Commit,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		StartedAt: d.startedAt,
		Host:      d.host,
		PID:       os.Getpid(),
		Channel:   "stable",
	}
}

// State returns the current daemon state.
func (d *Daemon) State() State {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// monitor periodically checks daemon health and logs warnings.
func (d *Daemon) monitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			health := d.Health()
			if health.MemAllocMB > 512 {
				d.MarkDegraded(fmt.Sprintf("high memory usage: %.1fMB", health.MemAllocMB))
			}
			if health.GoRoutines > 10000 {
				d.MarkDegraded(fmt.Sprintf("high goroutine count: %d", health.GoRoutines))
			}
			slog.Debug("Kaptaind health check",
				"state", health.State,
				"uptime", health.Uptime,
				"goroutines", health.GoRoutines,
				"mem_mb", fmt.Sprintf("%.1f", health.MemAllocMB),
				"workspaces", health.Workspaces,
			)
		}
	}
}

func formatUptime(d time.Duration) string {
	d = d.Truncate(time.Second)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
