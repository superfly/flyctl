// Package task implements async task handling.
package task

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
)

type contextKey struct{}

// WithContext derives a Context that carries the given Manager from ctx.
func WithContext(ctx context.Context, m Manager) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}

// FromContext returns the Manager ctx carries. It panics in case ctx carries
// no Manager.
func FromContext(ctx context.Context) Manager {
	return ctx.Value(contextKey{}).(Manager)
}

// NewWithContext derives a Context that carries a new Manager
func NewWithContext(ctx context.Context) context.Context {
	return WithContext(ctx, New())
}

// New initializes and returns a Manager which runs its tasks on the chain
// of the given parent context.
func New() Manager {
	return &manager{
		queue: make(chan Task, 10),
	}
}

// Task wraps the set of tasks.
type Task func(context.Context)

// Manager implements a task manager.
type Manager interface {
	pkg() // internal

	// Start begins running background tasks with the provided context.
	Start(context.Context)

	// Run enqueues the task to run in the background.
	Run(Task)

	// RunFinalizer enqueues the task to run in the background after Shutdown.
	RunFinalizer(Task)

	// Shutdown instructs all the tasks to shutdown and waits until they've done
	// so.
	Shutdown()

	// ShutdownWithTimeout instructs all the tasks to shutdown and waits up until the timeout for them to complete.
	ShutdownWithTimeout(time.Duration)
}

type manager struct {
	queue   chan Task
	started atomic.Bool
	sync.WaitGroup
}

func (*manager) pkg() {}

func (m *manager) Start(ctx context.Context) {
	log := logger.FromContext(ctx)

	ctx, cancel := context.WithCancel(ctx)

	started := m.started.Swap(true)
	if started {
		cancel()
		log.Debug("Task manager has already started; not starting again")
		return
	}

	go func() {
		defer cancel()

		log.Debug("Starting task manager")

		for t := range m.queue {
			t := t
			go func() {
				defer m.WaitGroup.Done()
				t(ctx)
			}()
		}

		log.Debug("Task manager done")
	}()
}

func (m *manager) Run(t Task) {
	m.WaitGroup.Add(1)
	m.queue <- t
}

func (m *manager) RunFinalizer(t Task) {
	m.Run(func(ctx context.Context) {
		// wait until the context is done before running the task
		<-ctx.Done()

		t(ctx)
	})
}

func (m *manager) Shutdown() {
	close(m.queue)
	started := m.started.Swap(true)
	if started {
		m.WaitGroup.Wait()
	}
}

func (m *manager) ShutdownWithTimeout(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{}, 1)
	go func() {
		m.Shutdown()
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		terminal.Debug("Shutdown timed out, exiting")
	case <-done:
	}
}
