// Package prompt implements input-related functionality.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/sort"
	"github.com/superfly/flyctl/internal/client"
)

func String(ctx context.Context, dst *string, msg, def string, required bool) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Input{
		Message: msg,
		Default: def,
	}

	opts := []survey.AskOpt{opt}
	if required {
		opts = append(opts, survey.WithValidator(survey.Required))
	}

	return survey.AskOne(p, dst, opts...)
}

func Password(ctx context.Context, dst *string, msg string, required bool) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Password{
		Message: msg,
	}

	opts := []survey.AskOpt{opt}
	if required {
		opts = append(opts, survey.WithValidator(survey.Required))
	}

	return survey.AskOne(p, dst, opts...)
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

func Confirmf(ctx context.Context, format string, a ...interface{}) (bool, error) {
	return Confirm(ctx, fmt.Sprintf(format, a...))
}

func Confirm(ctx context.Context, message string) (confirm bool, err error) {
	var opt survey.AskOpt
	if opt, err = newSurveyIO(ctx); err != nil {
		return
	}

	prompt := &survey.Confirm{
		Message: message,
	}

	err = survey.AskOne(prompt, &confirm, opt)

	return
}

var errNonInteractive = errors.New("prompt: non interactive")

func IsNonInteractive(err error) bool {
	return errors.Is(err, errNonInteractive)
}

type NonInteractiveError string

func (e NonInteractiveError) Error() string { return string(e) }

func (NonInteractiveError) Unwrap() error { return errNonInteractive }

func newSurveyIO(ctx context.Context) (survey.AskOpt, error) {
	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		return nil, errNonInteractive
	}

	in, ok := io.In.(terminal.FileReader)
	if !ok {
		return nil, errNonInteractive
	}

	out, ok := io.Out.(terminal.FileWriter)
	if !ok {
		return nil, errNonInteractive
	}

	return survey.WithStdio(in, out, io.ErrOut), nil
}

var errOrgSlugRequired = NonInteractiveError("org slug must be specified when not running interactively")

// Org returns the Organization the user has passed in via flag or prompts the
// user for one.
func Org(ctx context.Context, typ *api.OrganizationType) (*api.Organization, error) {
	client := client.FromContext(ctx).API()

	orgs, err := client.GetOrganizations(ctx, typ)
	if err != nil {
		return nil, err
	}
	sort.OrganizationsByTypeAndName(orgs)

	io := iostreams.FromContext(ctx)
	slug := config.FromContext(ctx).Organization

	switch {
	case slug == "" && len(orgs) == 1 && orgs[0].Type == "PERSONAL":
		fmt.Fprintf(io.ErrOut, "automatically selected %s organization: %s\n",
			strings.ToLower(orgs[0].Type), orgs[0].Name)

		return &orgs[0], nil
	case slug != "":
		for _, org := range orgs {
			if slug == org.Slug {
				return &org, nil
			}
		}

		return nil, fmt.Errorf("organization %s not found", slug)
	default:
		switch org, err := SelectOrg(ctx, orgs); {
		case err == nil:
			return org, nil
		case IsNonInteractive(err):
			return nil, errOrgSlugRequired
		default:
			return nil, err
		}
	}
}

func SelectOrg(ctx context.Context, orgs []api.Organization) (org *api.Organization, err error) {
	var options []string
	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	var index int
	if err = Select(ctx, &index, "Select Organization:", options...); err == nil {
		org = &orgs[index]
	}

	return
}
