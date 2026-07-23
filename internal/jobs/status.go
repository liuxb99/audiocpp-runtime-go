package jobs

import "fmt"

// Status constants for job lifecycle state machine.
const (
	StatusPending         Status = "pending"
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
	StatusCancelRequested Status = "cancel_requested"
	StatusCanceled        Status = "canceled"
	StatusRetryWaiting    Status = "retry_waiting"
	StatusTimedOut        Status = "timed_out"
)

// validStatuses records all valid status values.
var validStatuses = map[Status]bool{
	StatusPending:         true,
	StatusQueued:          true,
	StatusRunning:         true,
	StatusSucceeded:       true,
	StatusFailed:          true,
	StatusCancelRequested: true,
	StatusCanceled:        true,
	StatusRetryWaiting:    true,
	StatusTimedOut:        true,
}

// terminalStatuses are statuses that cannot transition to any other status.
var terminalStatuses = map[Status]bool{
	StatusSucceeded: true,
	StatusCanceled:  true,
	StatusTimedOut:  true,
}

// validTransitions maps current status → set of allowed next statuses.
var validTransitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusQueued:   true,
		StatusCanceled: true,
	},
	StatusQueued: {
		StatusRunning:  true,
		StatusCanceled: true,
	},
	StatusRunning: {
		StatusSucceeded:       true,
		StatusFailed:          true,
		StatusCancelRequested: true,
		StatusTimedOut:        true,
	},
	StatusCancelRequested: {
		StatusCanceled: true,
	},
	StatusFailed: {
		StatusRetryWaiting: true,
	},
	StatusRetryWaiting: {
		StatusQueued:   true,
		StatusCanceled: true,
	},
	// Terminal statuses have no outgoing transitions
	StatusSucceeded: {},
	StatusCanceled:  {},
	StatusTimedOut:  {},
}

func (s Status) IsValid() bool {
	return validStatuses[s]
}

func (s Status) IsTerminal() bool {
	return terminalStatuses[s]
}

// StatusTransitionError is returned when an illegal state transition is attempted.
type StatusTransitionError struct {
	JobID    string
	From, To Status
}

func (e *StatusTransitionError) Error() string {
	return fmt.Sprintf("job %s: illegal status transition from %q to %q", e.JobID, e.From, e.To)
}

// TransitionTo transitions the job to newStatus if the transition is valid.
// It returns an error if the transition is not allowed.
func (j *Job) TransitionTo(newStatus Status) error {
	if j.Status == "" {
		// Allow empty (new job) to transition to any valid initial status
		if !newStatus.IsValid() {
			return fmt.Errorf("invalid target status %q", newStatus)
		}
		j.Status = newStatus
		return nil
	}

	if j.Status.IsTerminal() {
		return &StatusTransitionError{
			JobID: j.ID,
			From:  j.Status,
			To:    newStatus,
		}
	}

	allowed, ok := validTransitions[j.Status]
	if !ok {
		return fmt.Errorf("job %s: unknown current status %q", j.ID, j.Status)
	}
	if !allowed[newStatus] {
		return &StatusTransitionError{
			JobID: j.ID,
			From:  j.Status,
			To:    newStatus,
		}
	}

	j.Status = newStatus
	return nil
}

// MustTransition panics if TransitionTo fails. Useful in tests.
func (j *Job) MustTransition(newStatus Status) {
	if err := j.TransitionTo(newStatus); err != nil {
		panic(err)
	}
}
