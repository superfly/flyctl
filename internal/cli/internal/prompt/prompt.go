// Package prompt implements input-related functionality.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/client"
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

var errNonInteractive = errors.New("non interactive")

func IsNonInteractive(err error) bool {
	return errors.Is(err, errNonInteractive)
}

func newSurveyIO(ctx context.Context) (opt survey.AskOpt, err error) {
	if io := iostreams.FromContext(ctx); !io.IsInteractive() {
		err = errNonInteractive
	} else {
		opt = survey.WithStdio(
			io.In.(terminal.FileReader),
			io.Out.(terminal.FileWriter),
			io.ErrOut,
		)
	}

	return
}

var errOrgSlugRequired = errors.New("org slug must be specified when not running interactively")

// Org returns the Organization the user has passed in via flag or prompts the
// user for one.
func Org(ctx context.Context, typ *api.OrganizationType) (*api.Organization, error) {
	client := client.FromContext(ctx).API()

	orgs, err := client.GetOrganizations(ctx, typ)
	if err != nil {
		return nil, err
	}
	sortOrgsByTypeAndName(orgs)

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
		switch org, err := selectOrg(ctx, orgs); {
		case err == nil:
			return org, nil
		case IsNonInteractive(err):
			return nil, errOrgSlugRequired
		default:
			return nil, err
		}
	}
}

func selectOrg(ctx context.Context, orgs []api.Organization) (org *api.Organization, err error) {
	var options []string
	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	var index int
	if err = Select(ctx, &index, "Select organization:", options...); err == nil {
		org = &orgs[index]
	}

	return
}

func sortOrgsByTypeAndName(orgs []api.Organization) {
	sort.Slice(orgs, func(i, j int) bool {
		return orgs[i].Type < orgs[j].Type && orgs[i].Name < orgs[j].Name
	})
}
