package spinner

import (
	"fmt"
	"sync"

	"github.com/superfly/flyctl/iostreams"
)

func Run(io *iostreams.IOStreams, msg string) (s *Spinner) {
	s = &Spinner{
		io:  io,
		msg: msg,
	}

	s.StartWithMessage(msg)

	return
}

type Spinner struct {
	mu  sync.Mutex
	io  *iostreams.IOStreams
	msg string
}

func (s *Spinner) Set(msg string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.msg
	s.msg = msg
	s.io.ChangeProgressIndicatorMsg(msg)

	return old
}

func (s *Spinner) Stop() string {
	return s.StopWithMessage("")
}

func (s *Spinner) StopWithSuccess() string {
	return s.StopWithMessage(fmt.Sprintf("%s %s", s.msg, s.io.ColorScheme().Green("✓")))
}

func (s *Spinner) StopWithMessage(msg string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.msg
	s.msg = msg
	s.io.StopProgressIndicatorMsg(msg)

	return old
}

func (s *Spinner) Start() {
	s.StartWithMessage("")
}

func (s *Spinner) StartWithMessage(msg string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.msg
	s.msg = msg
	s.io.StartProgressIndicatorMsg(msg)

	return old
}
