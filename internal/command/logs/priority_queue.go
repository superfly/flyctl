package logs

import "container/heap"

type minHeap[T any] struct {
	items []T
	less  func(a, b T) bool
}

func (h *minHeap[T]) Len() int           { return len(h.items) }
func (h *minHeap[T]) Less(i, j int) bool { return h.less(h.items[i], h.items[j]) }
func (h *minHeap[T]) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *minHeap[T]) Push(x any)         { h.items = append(h.items, x.(T)) }
func (h *minHeap[T]) Pop() any {
	n := len(h.items)
	item := h.items[n-1]
	h.items = h.items[:n-1]
	return item
}

type PriorityQueue[T any] struct {
	pq minHeap[T]
}

func NewPriorityQueue[T any](less func(a, b T) bool) *PriorityQueue[T] {
	pq := &PriorityQueue[T]{
		pq: minHeap[T]{
			less: less,
		},
	}
	heap.Init(&pq.pq)
	return pq
}

func (q *PriorityQueue[T]) Len() int { return q.pq.Len() }
func (q *PriorityQueue[T]) Push(o T) { heap.Push(&q.pq, o) }
func (q *PriorityQueue[T]) Pop() T   { return heap.Pop(&q.pq).(T) }
