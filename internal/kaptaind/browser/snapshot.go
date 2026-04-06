package browser

import "time"

type ActionType string

const (
	ActionTypeClick  ActionType = "click"
	ActionTypeType   ActionType = "type"
	ActionTypeSubmit ActionType = "submit"
	ActionTypeOpen   ActionType = "open"
)

type Action struct {
	Type   ActionType `json:"type"`
	Target string     `json:"target,omitempty"`
	Text   string     `json:"text,omitempty"`
}

type Snapshot struct {
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
	Engine    Engine    `json:"engine"`
	Elements  []Element `json:"elements"`
	Action    *Action   `json:"action,omitempty"`
	Escalated bool      `json:"escalated"`
}
