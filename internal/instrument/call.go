package instrument

import (
	"sync"
	"time"
)

var (
	mu         sync.Mutex
	GraphQL    CallInstrumenter
	Flaps      CallInstrumenter
	ApiAdapter = &ApiInstrumenter{Instrumenter: &GraphQL}
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

// adapter for the api package's instrumentation facade
type ApiInstrumenter struct {
	Instrumenter *CallInstrumenter
	current      *CallTimer
}

func (s *ApiInstrumenter) Begin() {
	if s.current != nil {
		panic("nested instrument span!")
	}

	timing := s.Instrumenter.Begin()
	s.current = &timing
}

func (s *ApiInstrumenter) End() {
	s.current.End()
	s.current = nil
}
