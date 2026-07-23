package jobs

import (
	"testing"
)

func TestStatusLifecycle_PendingToQueuedToRunningToSucceeded(t *testing.T) {
	job := &Job{ID: "test-1"}
	// Initial empty status → Pending
	if err := job.TransitionTo(StatusPending); err != nil {
		t.Fatalf("Pending transition failed: %v", err)
	}
	// Pending → Queued
	if err := job.TransitionTo(StatusQueued); err != nil {
		t.Fatalf("Queued transition failed: %v", err)
	}
	// Queued → Running
	if err := job.TransitionTo(StatusRunning); err != nil {
		t.Fatalf("Running transition failed: %v", err)
	}
	// Running → Succeeded
	if err := job.TransitionTo(StatusSucceeded); err != nil {
		t.Fatalf("Succeeded transition failed: %v", err)
	}
	if job.Status != StatusSucceeded {
		t.Errorf("expected final status %q, got %q", StatusSucceeded, job.Status)
	}
}

func TestStatusLifecycle_RunningToFailed(t *testing.T) {
	job := &Job{ID: "test-2"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)
	job.MustTransition(StatusRunning)

	if err := job.TransitionTo(StatusFailed); err != nil {
		t.Fatalf("Failed transition failed: %v", err)
	}
	if job.Status != StatusFailed {
		t.Errorf("expected %q, got %q", StatusFailed, job.Status)
	}
}

func TestStatusLifecycle_RunningToTimedOut(t *testing.T) {
	job := &Job{ID: "test-3"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)
	job.MustTransition(StatusRunning)

	if err := job.TransitionTo(StatusTimedOut); err != nil {
		t.Fatalf("TimedOut transition failed: %v", err)
	}
	if job.Status != StatusTimedOut {
		t.Errorf("expected %q, got %q", StatusTimedOut, job.Status)
	}
}

func TestStatusLifecycle_QueuedToCanceled(t *testing.T) {
	job := &Job{ID: "test-4"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)

	if err := job.TransitionTo(StatusCanceled); err != nil {
		t.Fatalf("Canceled transition failed: %v", err)
	}
	if job.Status != StatusCanceled {
		t.Errorf("expected %q, got %q", StatusCanceled, job.Status)
	}
}

func TestStatusLifecycle_RunningToCancelRequestedToCanceled(t *testing.T) {
	job := &Job{ID: "test-5"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)
	job.MustTransition(StatusRunning)

	if err := job.TransitionTo(StatusCancelRequested); err != nil {
		t.Fatalf("CancelRequested transition failed: %v", err)
	}
	if job.Status != StatusCancelRequested {
		t.Errorf("expected %q, got %q", StatusCancelRequested, job.Status)
	}

	if err := job.TransitionTo(StatusCanceled); err != nil {
		t.Fatalf("Canceled transition failed: %v", err)
	}
	if job.Status != StatusCanceled {
		t.Errorf("expected %q, got %q", StatusCanceled, job.Status)
	}
}

func TestStatusLifecycle_FailedToRetryWaitingToQueued(t *testing.T) {
	job := &Job{ID: "test-6"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)
	job.MustTransition(StatusRunning)
	job.MustTransition(StatusFailed)

	if err := job.TransitionTo(StatusRetryWaiting); err != nil {
		t.Fatalf("RetryWaiting transition failed: %v", err)
	}
	if job.Status != StatusRetryWaiting {
		t.Errorf("expected %q, got %q", StatusRetryWaiting, job.Status)
	}

	if err := job.TransitionTo(StatusQueued); err != nil {
		t.Fatalf("Queued transition from RetryWaiting failed: %v", err)
	}
	if job.Status != StatusQueued {
		t.Errorf("expected %q, got %q", StatusQueued, job.Status)
	}
}

func TestStatusLifecycle_SucceededToRunning_Illegal(t *testing.T) {
	job := &Job{ID: "test-7"}
	job.MustTransition(StatusPending)
	job.MustTransition(StatusQueued)
	job.MustTransition(StatusRunning)
	job.MustTransition(StatusSucceeded)

	err := job.TransitionTo(StatusRunning)
	if err == nil {
		t.Fatal("expected error for illegal transition from terminal state")
	}
	var ste *StatusTransitionError
	if !IsStatusTransitionError(err) {
		t.Errorf("expected StatusTransitionError, got %T", err)
	} else {
		ste = err.(*StatusTransitionError)
		if ste.JobID != "test-7" {
			t.Errorf("expected JobID 'test-7', got %q", ste.JobID)
		}
		if ste.From != StatusSucceeded {
			t.Errorf("expected From %q, got %q", StatusSucceeded, ste.From)
		}
		if ste.To != StatusRunning {
			t.Errorf("expected To %q, got %q", StatusRunning, ste.To)
		}
	}
}

func TestStatusLifecycle_TerminalStatesImmutable(t *testing.T) {
	terminalStates := []Status{StatusSucceeded, StatusCanceled, StatusTimedOut}

	for _, terminal := range terminalStates {
		job := &Job{ID: "test-terminal"}
		job.MustTransition(StatusPending)
		job.MustTransition(StatusQueued)
		job.MustTransition(StatusRunning)

		// Transition to the terminal state
		// Running→Canceled needs CancelRequested intermediate
		if terminal == StatusCanceled {
			job.MustTransition(StatusCancelRequested)
		}
		if err := job.TransitionTo(terminal); err != nil {
			t.Fatalf("failed to transition to %q: %v", terminal, err)
		}

		// Any further transition should fail
		if err := job.TransitionTo(StatusPending); err == nil {
			t.Errorf("expected error when transitioning from terminal state %q", terminal)
		}
		if err := job.TransitionTo(StatusQueued); err == nil {
			t.Errorf("expected error when transitioning from terminal state %q", terminal)
		}
		if err := job.TransitionTo(StatusRunning); err == nil {
			t.Errorf("expected error when transitioning from terminal state %q", terminal)
		}
	}
}

func TestStatusLifecycle_PendingToCanceled(t *testing.T) {
	job := &Job{ID: "test-8"}
	job.MustTransition(StatusPending)

	if err := job.TransitionTo(StatusCanceled); err != nil {
		t.Fatalf("Pending -> Canceled transition failed: %v", err)
	}
	if job.Status != StatusCanceled {
		t.Errorf("expected %q, got %q", StatusCanceled, job.Status)
	}
}

func TestStatusLifecycle_EmptyToPending(t *testing.T) {
	job := &Job{ID: "test-9"}
	// Empty status → Pending is allowed
	if err := job.TransitionTo(StatusPending); err != nil {
		t.Fatalf("Empty -> Pending transition failed: %v", err)
	}
}

func TestStatusLifecycle_EmptyToInvalid(t *testing.T) {
	job := &Job{ID: "test-10"}
	err := job.TransitionTo(Status("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid target status")
	}
}

// IsStatusTransitionError checks if an error is a StatusTransitionError.
func IsStatusTransitionError(err error) bool {
	_, ok := err.(*StatusTransitionError)
	return ok
}
