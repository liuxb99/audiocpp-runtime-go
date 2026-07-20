package jobs

import (
	"container/heap"
	"sync"
)

type queueItem struct {
	job       *Job
	index     int
	enqueueAt int64
}

type priorityHeap []*queueItem

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	if h[i].job.Priority != h[j].job.Priority {
		return h[i].job.Priority > h[j].job.Priority
	}
	return h[i].enqueueAt < h[j].enqueueAt
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*queueItem)
	item.index = n
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]
	return item
}

type Queue struct {
	mu      sync.Mutex
	heap    priorityHeap
	counter int64
}

func NewQueue() *Queue {
	return &Queue{
		heap: make(priorityHeap, 0),
	}
}

func (q *Queue) Enqueue(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.counter++
	item := &queueItem{
		job:       job,
		enqueueAt: q.counter,
	}
	heap.Push(&q.heap, item)
}

func (q *Queue) Dequeue() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	item := heap.Pop(&q.heap).(*queueItem)
	return item.job
}

func (q *Queue) Peek() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	return q.heap[0].job
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.heap.Len()
}

func (q *Queue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, item := range q.heap {
		if item.job.ID == id {
			heap.Remove(&q.heap, i)
			return true
		}
	}
	return false
}
