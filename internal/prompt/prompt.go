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
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/sort"
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

func Select(ctx context.Context, index *int, msg, def string, options ...string) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Select{
		Message:  msg,
		Options:  options,
		PageSize: 15,
	}

	if def != "" {
		p.Default = def
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

func ConfirmOverwrite(ctx context.Context, filename string) (confirm bool, err error) {
	prompt := &survey.Confirm{
		Message: fmt.Sprintf(`Overwrite "%s"?`, filename),
	}
	err = survey.AskOne(prompt, &confirm)

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
func Org(ctx context.Context) (*api.Organization, error) {
	client := client.FromContext(ctx).API()

	orgs, err := client.GetOrganizations(ctx)
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
		personalCallout := ""
		if org.Type == "PERSONAL" && org.Slug != "personal" {
			personalCallout = " [personal]"
		}
		options = append(options, fmt.Sprintf("%s (%s)%s", org.Name, org.Slug, personalCallout))
	}

	var index int
	if err = Select(ctx, &index, "Select Organization:", "", options...); err == nil {
		org = &orgs[index]
	}

	return
}

var errRegionSlugRequired = NonInteractiveError("region slug must be specified when not running interactively")

// Region returns the region the user has passed in via flag or prompts the
// user for one.
func Region(ctx context.Context) (*api.Region, error) {
	client := client.FromContext(ctx).API()

	regions, defaultRegion, err := client.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}
	sort.RegionsByNameAndCode(regions)

	slug := config.FromContext(ctx).Region

	switch {
	case slug != "":
		for _, region := range regions {
			if slug == region.Code {
				return &region, nil
			}
		}

		return nil, fmt.Errorf("region %s not found", slug)
	default:
		var defaultRegionCode string
		if defaultRegion != nil {
			defaultRegionCode = defaultRegion.Code
		}

		switch org, err := SelectRegion(ctx, regions, defaultRegionCode); {
		case err == nil:
			return org, nil
		case IsNonInteractive(err):
			return nil, errRegionSlugRequired
		default:
			return nil, err
		}
	}
}

func SelectRegion(ctx context.Context, regions []api.Region, defaultCode string) (region *api.Region, err error) {
	var defaultOption string

	var options []string
	for _, r := range regions {
		option := fmt.Sprintf("%s (%s)", r.Name, r.Code)
		if r.Code == defaultCode {
			defaultOption = option
		}

		options = append(options, option)
	}

	var index int
	if err = Select(ctx, &index, "Select region:", defaultOption, options...); err == nil {
		region = &regions[index]
	}

	return
}
