package jobs

import (
	"testing"
)

func TestEnqueueDequeue(t *testing.T) {
	q := NewQueue()
	job := &Job{ID: "job-1", Priority: 0}

	q.Enqueue(job)
	if q.Len() != 1 {
		t.Errorf("expected queue length 1, got %d", q.Len())
	}

	got := q.Dequeue()
	if got == nil {
		t.Fatal("expected non-nil job")
	}
	if got.ID != "job-1" {
		t.Errorf("expected job ID 'job-1', got %q", got.ID)
	}
	if q.Len() != 0 {
		t.Errorf("expected queue length 0 after dequeue, got %d", q.Len())
	}
}

func TestEnqueueDequeue_Multiple(t *testing.T) {
	q := NewQueue()
	jobs := []*Job{
		{ID: "job-a", Priority: 0},
		{ID: "job-b", Priority: 0},
		{ID: "job-c", Priority: 0},
	}

	for _, j := range jobs {
		q.Enqueue(j)
	}

	if q.Len() != 3 {
		t.Errorf("expected queue length 3, got %d", q.Len())
	}

	got1 := q.Dequeue()
	if got1.ID != "job-a" {
		t.Errorf("expected first dequeued job 'job-a', got %q", got1.ID)
	}
	got2 := q.Dequeue()
	if got2.ID != "job-b" {
		t.Errorf("expected second dequeued job 'job-b', got %q", got2.ID)
	}
	got3 := q.Dequeue()
	if got3.ID != "job-c" {
		t.Errorf("expected third dequeued job 'job-c', got %q", got3.ID)
	}
}

func TestPriority_HigherFirst(t *testing.T) {
	q := NewQueue()

	q.Enqueue(&Job{ID: "low", Priority: 1})
	q.Enqueue(&Job{ID: "high", Priority: 10})
	q.Enqueue(&Job{ID: "medium", Priority: 5})

	got1 := q.Dequeue()
	if got1.ID != "high" {
		t.Errorf("expected first dequeued to be 'high' (priority 10), got %q", got1.ID)
	}
	got2 := q.Dequeue()
	if got2.ID != "medium" {
		t.Errorf("expected second dequeued to be 'medium' (priority 5), got %q", got2.ID)
	}
	got3 := q.Dequeue()
	if got3.ID != "low" {
		t.Errorf("expected third dequeued to be 'low' (priority 1), got %q", got3.ID)
	}
}

func TestPriority_SamePriorityFIFO(t *testing.T) {
	q := NewQueue()

	q.Enqueue(&Job{ID: "first", Priority: 5})
	q.Enqueue(&Job{ID: "second", Priority: 5})
	q.Enqueue(&Job{ID: "third", Priority: 5})

	got1 := q.Dequeue()
	if got1.ID != "first" {
		t.Errorf("expected 'first', got %q", got1.ID)
	}
	got2 := q.Dequeue()
	if got2.ID != "second" {
		t.Errorf("expected 'second', got %q", got2.ID)
	}
	got3 := q.Dequeue()
	if got3.ID != "third" {
		t.Errorf("expected 'third', got %q", got3.ID)
	}
}

func TestPriority_Mixed(t *testing.T) {
	q := NewQueue()

	q.Enqueue(&Job{ID: "a", Priority: 1})
	q.Enqueue(&Job{ID: "b", Priority: 3})
	q.Enqueue(&Job{ID: "c", Priority: 2})
	q.Enqueue(&Job{ID: "d", Priority: 3})
	q.Enqueue(&Job{ID: "e", Priority: 1})

	got1 := q.Dequeue()
	if got1.ID != "b" {
		t.Errorf("expected 'b' (priority 3), got %q", got1.ID)
	}
	got2 := q.Dequeue()
	if got2.ID != "d" {
		t.Errorf("expected 'd' (priority 3, FIFO), got %q", got2.ID)
	}
	got3 := q.Dequeue()
	if got3.ID != "c" {
		t.Errorf("expected 'c' (priority 2), got %q", got3.ID)
	}
	got4 := q.Dequeue()
	if got4.ID != "a" {
		t.Errorf("expected 'a' (priority 1, FIFO), got %q", got4.ID)
	}
	got5 := q.Dequeue()
	if got5.ID != "e" {
		t.Errorf("expected 'e' (priority 1, FIFO), got %q", got5.ID)
	}
}

func TestRemove(t *testing.T) {
	q := NewQueue()

	q.Enqueue(&Job{ID: "job-1", Priority: 1})
	q.Enqueue(&Job{ID: "job-2", Priority: 2})
	q.Enqueue(&Job{ID: "job-3", Priority: 3})

	if !q.Remove("job-2") {
		t.Error("expected Remove('job-2') to return true")
	}
	if q.Len() != 2 {
		t.Errorf("expected queue length 2 after removal, got %d", q.Len())
	}

	got1 := q.Dequeue()
	if got1.ID != "job-3" {
		t.Errorf("expected 'job-3' to be first, got %q", got1.ID)
	}
	got2 := q.Dequeue()
	if got2.ID != "job-1" {
		t.Errorf("expected 'job-1' to be second, got %q", got2.ID)
	}
}

func TestRemove_NonExistent(t *testing.T) {
	q := NewQueue()
	q.Enqueue(&Job{ID: "job-1", Priority: 1})

	if q.Remove("nonexistent") {
		t.Error("expected Remove('nonexistent') to return false")
	}
	if q.Len() != 1 {
		t.Errorf("expected queue length 1, got %d", q.Len())
	}
}

func TestRemove_FromEmpty(t *testing.T) {
	q := NewQueue()
	if q.Remove("nothing") {
		t.Error("expected Remove from empty queue to return false")
	}
}

func TestRemove_LastItem(t *testing.T) {
	q := NewQueue()
	q.Enqueue(&Job{ID: "only", Priority: 1})

	if !q.Remove("only") {
		t.Error("expected Remove('only') to return true")
	}
	if q.Len() != 0 {
		t.Errorf("expected queue length 0, got %d", q.Len())
	}
}

func TestEmpty(t *testing.T) {
	q := NewQueue()

	if q.Len() != 0 {
		t.Errorf("expected new queue length 0, got %d", q.Len())
	}
	got := q.Dequeue()
	if got != nil {
		t.Errorf("expected nil from empty queue, got %v", got)
	}
	got = q.Peek()
	if got != nil {
		t.Errorf("expected nil Peek from empty queue, got %v", got)
	}
}

func TestPeek_DoesNotRemove(t *testing.T) {
	q := NewQueue()
	q.Enqueue(&Job{ID: "peek-me", Priority: 5})

	got := q.Peek()
	if got == nil {
		t.Fatal("expected non-nil Peek")
	}
	if got.ID != "peek-me" {
		t.Errorf("expected 'peek-me', got %q", got.ID)
	}
	if q.Len() != 1 {
		t.Errorf("expected queue length 1 after Peek, got %d", q.Len())
	}
}

func TestLen_AfterOperations(t *testing.T) {
	q := NewQueue()

	if q.Len() != 0 {
		t.Errorf("expected 0, got %d", q.Len())
	}
	q.Enqueue(&Job{ID: "a", Priority: 1})
	if q.Len() != 1 {
		t.Errorf("expected 1, got %d", q.Len())
	}
	q.Enqueue(&Job{ID: "b", Priority: 2})
	if q.Len() != 2 {
		t.Errorf("expected 2, got %d", q.Len())
	}
	q.Dequeue()
	if q.Len() != 1 {
		t.Errorf("expected 1 after dequeue, got %d", q.Len())
	}
	q.Dequeue()
	if q.Len() != 0 {
		t.Errorf("expected 0 after all dequeues, got %d", q.Len())
	}
}
