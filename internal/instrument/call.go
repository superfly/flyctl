package instrument

import (
	"sync"
	"time"
)

var (
	mu         sync.Mutex
	GraphQL    CallInstrumenter
	Flaps      CallInstrumenter
	ApiAdapter = &ApiInstrumenter{metrics: &GraphQL.metrics}
)

type CallInstrumenter struct {
	metrics CallMetrics
}

type CallMetrics struct {
	Calls    int
	Duration float64
}

type CallTimer struct {
	Start   time.Time
	metrics *CallMetrics
}

func (i *CallInstrumenter) Begin() CallTimer {
	return CallTimer{
		Start:   time.Now(),
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

	duration := time.Since(t.Start).Seconds()

	t.metrics.Calls += 1
	t.metrics.Duration += duration
}

// adapter for the api package's instrumentation facade
type ApiInstrumenter struct {
	metrics *CallMetrics
}

func (i *ApiInstrumenter) ReportCallTiming(duration time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	i.metrics.Calls += 1
	i.metrics.Duration += duration.Seconds()
}
