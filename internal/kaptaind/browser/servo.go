package browser

import (
	"errors"
)

// ServoRuntime is a full DOM engine based on Servo
type ServoRuntime struct {
	sessionID string
	state     StructuredState
}

// NewServoRuntime creates a new Servo-based WebRuntime
func NewServoRuntime(sessionID string) *ServoRuntime {
	return &ServoRuntime{
		sessionID: sessionID,
		state: StructuredState{
			SessionID: sessionID,
			Engine:    EngineServo,
		},
	}
}

func (r *ServoRuntime) Open(url string) (StructuredState, error) {
	// TODO: FFI to Servo
	r.state.URL = url
	r.state.Elements = []Element{}
	return r.state, nil
}

func (r *ServoRuntime) Click(targetID string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *ServoRuntime) Type(targetID string, text string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *ServoRuntime) Submit(targetID *string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *ServoRuntime) Back() (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *ServoRuntime) Refresh() (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *ServoRuntime) Extract() (StructuredState, error) {
	return r.state, nil
}
