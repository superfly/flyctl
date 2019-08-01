package terminal

import (
	"time"

	"github.com/briandowns/spinner"
)

type ProgressFn func() error

func WithProgressE(message string, fn func() error) error {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.FinalMSG = message + ", done\n"
	s.Prefix = message + " "
	s.Start()
	err := fn()
	if err != nil {
		s.FinalMSG = message + ", error\n"
	}
	s.Stop()
	return err
}

func WithProgress(message string, fn func()) {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.FinalMSG = message + ", done\n"
	s.Prefix = message + " "
	s.Start()
	fn()
	s.Stop()
}
