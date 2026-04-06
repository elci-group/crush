package browser

// Coordinator manages the active WebRuntime and handles escalation between engines
type Coordinator struct {
	sessionID string
	active    WebRuntime
	text      *W3MRuntime
	servo     *ServoRuntime
}

func NewCoordinator(sessionID string) *Coordinator {
	textEngine := NewW3MRuntime(sessionID)
	return &Coordinator{
		sessionID: sessionID,
		text:      textEngine,
		active:    textEngine, // Default to text-first
	}
}

func (c *Coordinator) EscalateToServo() error {
	if c.servo == nil {
		c.servo = NewServoRuntime(c.sessionID)
	}

	// Preserve state continuity (URL, etc.)
	prevState, err := c.active.Extract()
	if err == nil && prevState.URL != "" {
		_, _ = c.servo.Open(prevState.URL)
	}

	c.active = c.servo
	return nil
}

// ActiveRuntime returns the currently active runtime
func (c *Coordinator) ActiveRuntime() WebRuntime {
	return c.active
}

func (c *Coordinator) ExecuteWithFallback(action func(WebRuntime) (StructuredState, error)) (StructuredState, error) {
	state, err := action(c.active)
	if err != nil {
		return state, err
	}

	// Check if escalation is required
	var requiresJS bool
	if state.Metadata.RequiresJS != nil {
		requiresJS = *state.Metadata.RequiresJS
	}

	if requiresJS && state.Engine == EngineText {
		if escErr := c.EscalateToServo(); escErr == nil {
			// Retry action with Servo
			return action(c.active)
		}
	}

	return state, nil
}
