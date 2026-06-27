package logs

import "container/heap"

type Comparable[T any] interface{ Less(other T) bool }

type MinHeap[T Comparable[T]] []T

func (h *MinHeap[T]) Len() int           { return len(*h) }
func (h *MinHeap[T]) Less(i, j int) bool { return (*h)[i].Less((*h)[j]) }
func (h *MinHeap[T]) Swap(i, j int)      { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }
func (h *MinHeap[T]) Push(x any)         { *h = append(*h, x.(T)) }
func (h *MinHeap[T]) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (h *MinHeap[T]) Insert(item T) { heap.Push(h, item) }
func (h *MinHeap[T]) PopMin() T     { return heap.Pop(h).(T) }
func NewMinHeap[T Comparable[T]](objects []T) MinHeap[T] {
	h := MinHeap[T](objects)
	heap.Init(&h)
	return h
}
