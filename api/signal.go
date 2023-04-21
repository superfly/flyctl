package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"syscall"
)

type Signal struct {
	syscall.Signal
}

func (s *Signal) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Signal)
}

func (s *Signal) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	if v == nil {
		s.Signal = 0
		return nil
	}

	switch value := v.(type) {
	case float64:
		s.Signal = syscall.Signal(value)
		return nil
	case string:
		sig, ok := linuxSignals[value]
		if !ok {
			return fmt.Errorf("unrecognized signal name '%s'", value)
		}
		s.Signal = sig
		return nil
	default:
		return errors.New("invalid signal")
	}
}

func NewSignal(name string) *Signal {
	sig, ok := linuxSignals[name]
	if !ok {
		return nil
	}
	return &Signal{Signal: sig}
}

var linuxSignals = map[string]syscall.Signal{
	"SIGABRT": syscall.SIGABRT,
	"SIGALRM": syscall.SIGALRM,
	"SIGFPE":  syscall.SIGFPE,
	"SIGHUP":  syscall.SIGHUP,
	"SIGILL":  syscall.SIGILL,
	"SIGINT":  syscall.SIGINT,
	"SIGKILL": syscall.SIGKILL,
	"SIGPIPE": syscall.SIGPIPE,
	"SIGQUIT": syscall.SIGQUIT,
	"SIGSEGV": syscall.SIGSEGV,
	"SIGTERM": syscall.SIGTERM,
	"SIGTRAP": syscall.SIGTRAP,
	"SIGUSR1": syscall.Signal(0xa), // SIGUSR1 Doesn't exist on windows
}
