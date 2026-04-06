package browser

type Engine string

const (
	EngineText  Engine = "text"
	EngineServo Engine = "servo"
)

type Role string

const (
	RoleLink   Role = "link"
	RoleButton Role = "button"
	RoleInput  Role = "input"
	RoleText   Role = "text"
)

type Position struct {
	Line        *int  `json:"line,omitempty"`
	BoundingBox []int `json:"boundingBox,omitempty"`
}

type Element struct {
	ID         string    `json:"id"`
	Role       Role      `json:"role"`
	Label      string    `json:"label"`
	Actionable bool      `json:"actionable"`
	Value      *string   `json:"value,omitempty"`
	Href       *string   `json:"href,omitempty"`
	Position   *Position `json:"position,omitempty"`
}

type Metadata struct {
	Title           *string `json:"title,omitempty"`
	RequiresJS      *bool   `json:"requires_js,omitempty"`
	ConfidenceScore float64 `json:"confidence_score"`
}

type StructuredState struct {
	SessionID string    `json:"session_id"`
	URL       string    `json:"url"`
	Engine    Engine    `json:"engine"`
	Elements  []Element `json:"elements"`
	Metadata  Metadata  `json:"metadata"`
}

type WebRuntime interface {
	Open(url string) (StructuredState, error)
	Click(targetID string) (StructuredState, error)
	Type(targetID string, text string) (StructuredState, error)
	Submit(targetID *string) (StructuredState, error)
	Back() (StructuredState, error)
	Refresh() (StructuredState, error)
	Extract() (StructuredState, error)
}
