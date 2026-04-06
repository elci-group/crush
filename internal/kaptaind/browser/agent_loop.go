package browser

import (
	"log/slog"
	"time"
)

// AgentLoop is an example of the agent interaction loop for Kaptaind
type AgentLoop struct {
	Coordinator *Coordinator
	Snapshots   []Snapshot
}

func NewAgentLoop(sessionID string) *AgentLoop {
	return &AgentLoop{
		Coordinator: NewCoordinator(sessionID),
		Snapshots:   []Snapshot{},
	}
}

func (l *AgentLoop) logSnapshot(state StructuredState, action *Action, escalated bool) {
	snapshot := Snapshot{
		Timestamp: time.Now(),
		URL:       state.URL,
		Engine:    state.Engine,
		Elements:  state.Elements,
		Action:    action,
		Escalated: escalated,
	}
	l.Snapshots = append(l.Snapshots, snapshot)
}

func (l *AgentLoop) ExecuteDecision(action Action) (StructuredState, error) {
	startEngine := l.Coordinator.ActiveRuntime().(*W3MRuntime) // Simplified tracking
	wasText := startEngine != nil                              // simplified
	_ = wasText

	state, err := l.Coordinator.ExecuteWithFallback(func(runtime WebRuntime) (StructuredState, error) {
		switch action.Type {
		case ActionTypeOpen:
			return runtime.Open(action.Text)
		case ActionTypeClick:
			return runtime.Click(action.Target)
		case ActionTypeType:
			return runtime.Type(action.Target, action.Text)
		case ActionTypeSubmit:
			var target *string
			if action.Target != "" {
				target = &action.Target
			}
			return runtime.Submit(target)
		}
		return runtime.Extract()
	})

	if err != nil {
		slog.Error("Failed to execute action", "error", err)
		return state, err
	}

	escalated := state.Engine == EngineServo
	l.logSnapshot(state, &action, escalated)

	return state, nil
}
