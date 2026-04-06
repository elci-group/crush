package browser

import (
	"errors"
)

// W3MRuntime is a text-first engine based on w3m
type W3MRuntime struct {
	sessionID string
	state     StructuredState
}

// NewW3MRuntime creates a new W3M-based WebRuntime
func NewW3MRuntime(sessionID string) *W3MRuntime {
	return &W3MRuntime{
		sessionID: sessionID,
		state: StructuredState{
			SessionID: sessionID,
			Engine:    EngineText,
		},
	}
}

func (r *W3MRuntime) Open(url string) (StructuredState, error) {
	// TODO: actually spawn w3m process and extract state
	r.state.URL = url
	r.state.Elements = []Element{}
	return r.state, nil
}

func (r *W3MRuntime) Click(targetID string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *W3MRuntime) Type(targetID string, text string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *W3MRuntime) Submit(targetID *string) (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *W3MRuntime) Back() (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *W3MRuntime) Refresh() (StructuredState, error) {
	return r.state, errors.New("not implemented")
}

func (r *W3MRuntime) Extract() (StructuredState, error) {
	return r.state, nil
}
