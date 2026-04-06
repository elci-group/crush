package model

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/crush/internal/agent/delegation"
	"github.com/charmbracelet/crush/internal/ui/chat"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

// SubAgentChatWindow manages a single sub-agent's chat window.
type SubAgentChatWindow struct {
	TaskID      string
	Task        *delegation.SubTask
	Chat        *Chat
	ExecutionID string

	// Window dimensions (relative to parent)
	startY int
	height int
	width  int

	// Message tracking
	messages []chat.MessageItem
	mu       sync.RWMutex
}

// NewSubAgentChatWindow creates a new sub-agent chat window.
func NewSubAgentChatWindow(com *common.Common, task *delegation.SubTask) *SubAgentChatWindow {
	return &SubAgentChatWindow{
		TaskID:   task.ID,
		Task:     task,
		Chat:     NewChat(com),
		messages: []chat.MessageItem{},
	}
}

// AppendMessage adds a message to the sub-agent's chat.
func (w *SubAgentChatWindow) AppendMessage(msg chat.MessageItem) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append(w.messages, msg)
	w.Chat.AppendMessages(msg)
}

// SetSize sets the window dimensions.
func (w *SubAgentChatWindow) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.Chat.SetSize(width, height)
}

// SetPosition sets the Y offset for rendering.
func (w *SubAgentChatWindow) SetPosition(startY int) {
	w.startY = startY
}

// Draw renders the window at its designated position.
func (w *SubAgentChatWindow) Draw(scr uv.Screen, parentWidth, parentHeight int) {
	// Create rectangle for this window's area
	area := uv.Rect(0, w.startY, parentWidth, w.height)
	w.Chat.Draw(scr, area)
}

// SubAgentWindowManager manages multiple concurrent sub-agent windows.
type SubAgentWindowManager struct {
	windows map[string]*SubAgentChatWindow
	order   []string // task IDs in order
	mu      sync.RWMutex

	// Layout info
	totalHeight int
	totalWidth  int
}

// NewSubAgentWindowManager creates a new manager for sub-agent windows.
func NewSubAgentWindowManager() *SubAgentWindowManager {
	return &SubAgentWindowManager{
		windows: make(map[string]*SubAgentChatWindow),
		order:   []string{},
	}
}

// AddWindow adds a new sub-agent window.
func (m *SubAgentWindowManager) AddWindow(com *common.Common, task *delegation.SubTask) *SubAgentChatWindow {
	m.mu.Lock()
	defer m.mu.Unlock()

	window := NewSubAgentChatWindow(com, task)
	m.windows[task.ID] = window
	m.order = append(m.order, task.ID)
	m.recalculateLayout()
	return window
}

// RemoveWindow removes a sub-agent window.
func (m *SubAgentWindowManager) RemoveWindow(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.windows, taskID)
	for i, id := range m.order {
		if id == taskID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.recalculateLayout()
}

// GetWindow retrieves a window by task ID.
func (m *SubAgentWindowManager) GetWindow(taskID string) *SubAgentChatWindow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.windows[taskID]
}

// GetActiveCount returns the number of active windows.
func (m *SubAgentWindowManager) GetActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.windows)
}

// SetSize sets the total available area and recalculates window layouts.
func (m *SubAgentWindowManager) SetSize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalWidth = width
	m.totalHeight = height
	m.recalculateLayout()
}

// recalculateLayout updates all window positions and sizes based on 1/(1+S) allocation.
// S = number of sub-agents.
func (m *SubAgentWindowManager) recalculateLayout() {
	s := len(m.windows)
	if s == 0 {
		return
	}

	// Each agent gets 1/(1+S) of the total height
	agentHeightAlloc := int(float64(m.totalHeight) / float64(1+s))
	if agentHeightAlloc < 3 {
		agentHeightAlloc = 3 // minimum viable height
	}

	currentY := 0
	for _, taskID := range m.order {
		window, ok := m.windows[taskID]
		if !ok {
			continue
		}

		window.SetPosition(currentY)
		window.SetSize(m.totalWidth, agentHeightAlloc)
		currentY += agentHeightAlloc + 1 // +1 for separator line
	}
}

// RenderAll renders all windows sequentially.
func (m *SubAgentWindowManager) RenderAll(scr uv.Screen) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, taskID := range m.order {
		window, ok := m.windows[taskID]
		if !ok {
			continue
		}

		window.Draw(scr, m.totalWidth, m.totalHeight)

		// TODO: Draw separator between windows if needed
		// if window.startY+window.height < m.totalHeight {
		//     // render separator line
		// }
	}
}

// GetLayout returns information about current window layout.
func (m *SubAgentWindowManager) GetLayout() SubAgentLayout {
	m.mu.RLock()
	defer m.mu.RUnlock()

	layout := SubAgentLayout{
		TotalWindows: len(m.windows),
		Windows:      []SubAgentWindowInfo{},
	}

	for _, taskID := range m.order {
		window, ok := m.windows[taskID]
		if !ok {
			continue
		}

		info := SubAgentWindowInfo{
			TaskID:   taskID,
			Title:    window.Task.Title,
			Provider: window.Task.AssignedProvider,
			StartY:   window.startY,
			Height:   window.height,
			Width:    window.width,
		}
		layout.Windows = append(layout.Windows, info)
	}

	return layout
}

// SubAgentLayout holds layout information for rendering.
type SubAgentLayout struct {
	TotalWindows int
	Windows      []SubAgentWindowInfo
}

// SubAgentWindowInfo describes a single window's layout.
type SubAgentWindowInfo struct {
	TaskID   string
	Title    string
	Provider string
	StartY   int
	Height   int
	Width    int
}

// DelegationUIState tracks delegation-related UI state.
type DelegationUIState struct {
	Coordinator      *delegation.Coordinator
	WindowManager    *SubAgentWindowManager
	DelegationActive bool
	ShowDelegation   bool

	mu sync.RWMutex
}

// NewDelegationUIState creates a new delegation UI state.
func NewDelegationUIState() *DelegationUIState {
	return &DelegationUIState{
		WindowManager:    NewSubAgentWindowManager(),
		DelegationActive: false,
		ShowDelegation:   false,
	}
}

// StartDelegation initializes delegation mode with the given plan.
func (d *DelegationUIState) StartDelegation(com *common.Common, plan *delegation.DelegationPlan) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.DelegationActive {
		return fmt.Errorf("delegation already active")
	}

	// Create coordinator
	coordinator := delegation.NewCoordinator(plan, "main_agent")
	d.Coordinator = coordinator
	d.DelegationActive = true

	// Create windows for each sub-task
	for _, task := range plan.SubTasks {
		d.WindowManager.AddWindow(com, &task)
	}

	return nil
}

// EndDelegation stops delegation mode.
func (d *DelegationUIState) EndDelegation() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Coordinator != nil {
		d.Coordinator.Cancel()
	}
	d.DelegationActive = false
	d.WindowManager = NewSubAgentWindowManager()
	d.Coordinator = nil
}

// IsActive returns whether delegation is currently active.
func (d *DelegationUIState) IsActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.DelegationActive
}

// CalculateMainChatHeight returns the height allocated for the main chat window.
// Given S active sub-agents, main chat gets 1/(1+S) of available height.
func (d *DelegationUIState) CalculateMainChatHeight(totalHeight int) int {
	d.mu.RLock()
	count := d.WindowManager.GetActiveCount()
	d.mu.RUnlock()

	if count == 0 {
		return totalHeight
	}

	// Main chat gets 1/(1+S) where S is number of sub-agents
	return int(float64(totalHeight) / float64(1+count))
}

// CalculateSubAgentChatHeight returns the height allocated for each sub-agent.
func (d *DelegationUIState) CalculateSubAgentChatHeight(totalHeight int) int {
	d.mu.RLock()
	count := d.WindowManager.GetActiveCount()
	d.mu.RUnlock()

	if count == 0 {
		return 0
	}

	// Each sub-agent gets 1/(1+S) of available height
	return int(float64(totalHeight) / float64(1+count))
}
