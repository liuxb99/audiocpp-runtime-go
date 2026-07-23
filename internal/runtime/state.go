package runtime

import "fmt"

type RuntimeState int32

const (
	StateCreated      RuntimeState = 0
	StateInitializing RuntimeState = 1
	StateStarting     RuntimeState = 2
	StateReady        RuntimeState = 3
	StateRunning      RuntimeState = 4
	StateStopping     RuntimeState = 5
	StateStopped      RuntimeState = 6
)

var validTransitions = map[RuntimeState][]RuntimeState{
	StateCreated:      {StateInitializing, StateStopping},
	StateInitializing: {StateStarting, StateStopping},
	StateStarting:     {StateReady, StateStopping},
	StateReady:        {StateRunning, StateStopping},
	StateRunning:      {StateStopping},
	StateStopping:     {StateStopped},
	StateStopped:      {},
}

func (r *Runtime) transition(from, to RuntimeState) error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.state != from {
		return fmt.Errorf("invalid transition: current state is %s, expected %s",
			StateString(r.state), StateString(from))
	}

	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions defined from state %s", StateString(from))
	}

	valid := false
	for _, s := range allowed {
		if s == to {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s -> %s", StateString(from), StateString(to))
	}

	r.state = to
	return nil
}

// transitionLocked is like transition but assumes stateMu is already held.
func (r *Runtime) transitionLocked(from, to RuntimeState) error {
	if r.state != from {
		return fmt.Errorf("invalid transition: current state is %s, expected %s",
			StateString(r.state), StateString(from))
	}

	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions defined from state %s", StateString(from))
	}

	valid := false
	for _, s := range allowed {
		if s == to {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s -> %s", StateString(from), StateString(to))
	}

	r.state = to
	return nil
}

func (r *Runtime) currentState() RuntimeState {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.state
}

func (r *Runtime) CurrentState() RuntimeState {
	return r.currentState()
}

func StateString(s RuntimeState) string {
	switch s {
	case StateCreated:
		return "created"
	case StateInitializing:
		return "initializing"
	case StateStarting:
		return "starting"
	case StateReady:
		return "ready"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return fmt.Sprintf("unknown(%d)", int32(s))
	}
}
