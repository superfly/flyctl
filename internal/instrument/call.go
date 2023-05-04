package instrument

import (
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	GraphQL CallInstrumenter
	Flaps   CallInstrumenter
)

type CallInstrumenter struct {
	metrics CallMetrics
}

type CallMetrics struct {
	Calls    int
	Duration float64
}

type CallTimer struct {
	start   time.Time
	metrics *CallMetrics
}

func (i *CallInstrumenter) Begin() CallTimer {
	return CallTimer{
		start:   time.Now(),
		metrics: &i.metrics,
	}
}

func (i *CallInstrumenter) Get() CallMetrics {
	mu.Lock()
	defer mu.Unlock()

	return i.metrics
}

func (t *CallTimer) End() {
	mu.Lock()
	defer mu.Unlock()

	duration := time.Since(t.start).Seconds()

	t.metrics.Calls += 1
	t.metrics.Duration += duration
}
