// Package task implements async task handling.
package task

import (
	"context"
	"sync"
)

type contextKey struct{}

// NewContext derives a Context that carries the given Manager from ctx.
func NewContext(ctx context.Context, m Manager) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}

// FromContext returns the Manager ctx carries. It panics in case ctx carries
// no Manager.
func FromContext(ctx context.Context) Manager {
	return ctx.Value(contextKey{}).(Manager)
}

// New initializes and returns a Manager which runs its tasks on the chain
// of the given parent context.
func New(parent context.Context) Manager {
	ctx, cancel := context.WithCancel(parent)

	return &manager{
		Context:    ctx,
		CancelFunc: cancel,
	}
}

// Task wraps the set of tasks.
type Task func(context.Context)

// Manager implements a task manager.
type Manager interface {
	pkg() // internal

	// Run runs the task in its own goroutine.
	Run(Task)

	// Shutdown instructs all the tasks to shutdown and waits until they've done
	// so.
	Shutdown()
}

type manager struct {
	context.Context
	context.CancelFunc
	sync.WaitGroup
}

func (*manager) pkg() {}

func (m *manager) Run(t Task) {
	m.WaitGroup.Add(1)

	go func() {
		defer m.WaitGroup.Done()

		t(m.Context)
	}()
}

func (m *manager) Shutdown() {
	m.CancelFunc()
	m.WaitGroup.Wait()
}
