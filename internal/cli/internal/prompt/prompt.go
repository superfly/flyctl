// Package prompt implements input-related functionality.
package prompt

import (
	"context"
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/superfly/flyctl/pkg/iostreams"
)

func String(ctx context.Context, dst *string, msg, def string) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Input{
		Message: msg,
		Default: def,
	}

	return survey.AskOne(p, dst, opt)
}

func Select(ctx context.Context, index *int, msg string, options ...string) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Select{
		Message:  msg,
		Options:  options,
		PageSize: 15,
	}

	return survey.AskOne(p, index, opt)
}

var errNonInteractive = errors.New("non interactive")

func IsNonInteractive(err error) bool {
	return errors.Is(err, errNonInteractive)
}

func newSurveyIO(ctx context.Context) (opt survey.AskOpt, err error) {
	switch io := iostreams.FromContext(ctx); io.CanPrompt() {
	default:
		err = errNonInteractive
	case true:
		opt = survey.WithStdio(
			io.In.(terminal.FileReader),
			io.Out.(terminal.FileWriter),
			io.ErrOut,
		)
	}

	return
}
