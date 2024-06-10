// Package prompt implements input-related functionality.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/AlecAivazis/survey/v2"
	surveyCore "github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/mgutz/ansi"
	"github.com/samber/lo"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/future"
	"github.com/superfly/flyctl/internal/sort"
	"github.com/superfly/flyctl/iostreams"
)

type RegionParams struct {
	Message             string
	ExcludedRegionCodes []string
}

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

func StringWithHelp(ctx context.Context, dst *string, msg, def, help string, required bool) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Input{
		Message: msg,
		Help:    help,
		Default: def,
	}

	opts := []survey.AskOpt{opt}
	if required {
		opts = append(opts, survey.WithValidator(survey.Required))
	}
	return survey.AskOne(p, dst, opts...)
}

func Int(ctx context.Context, dst *int, msg string, def int, required bool) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.Input{
		Message: msg,
		Default: strconv.Itoa(def),
	}

	opts := []survey.AskOpt{opt}
	if required {
		opts = append(opts, survey.WithValidator(survey.Required))
	}
	// add a validator to ensure that the input is an integer
	opts = append(opts, survey.WithValidator(func(val interface{}) error {
		_, err := strconv.Atoi(val.(string))
		if err != nil {
			return errors.New("must be an integer")
		}
		return nil
	}))
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

func MultiSelect(ctx context.Context, indices *[]int, msg string, def []int, options ...string) error {
	opt, err := newSurveyIO(ctx)
	if err != nil {
		return err
	}

	p := &survey.MultiSelect{
		Message:  msg,
		Options:  options,
		PageSize: 15,
		Default:  def,
	}

	return survey.AskOne(p, indices, opt)
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

func ConfirmYes(ctx context.Context, message string) (confirm bool, err error) {
	var opt survey.AskOpt
	if opt, err = newSurveyIO(ctx); err != nil {
		return
	}

	prompt := &survey.Confirm{
		Message: message,
		Default: true,
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

var ErrNonInteractive = errors.New("prompt: non interactive")

func IsNonInteractive(err error) bool {
	return errors.Is(err, ErrNonInteractive)
}

type NonInteractiveError string

func (e NonInteractiveError) Error() string { return string(e) }

func (NonInteractiveError) Unwrap() error { return ErrNonInteractive }

func isInteractive(ctx context.Context) bool {
	io := iostreams.FromContext(ctx)
	return io.IsInteractive()
}

func newSurveyIO(ctx context.Context) (survey.AskOpt, error) {
	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		return nil, ErrNonInteractive
	}

	in, ok := io.In.(terminal.FileReader)
	if !ok {
		return nil, ErrNonInteractive
	}

	out, ok := io.Out.(terminal.FileWriter)
	if !ok {
		return nil, ErrNonInteractive
	}

	surveyCore.TemplateFuncsWithColor["color"] = func(style string) string {
		switch style {
		case "white":
			return ansi.ColorCode("default")
		default:
			return ansi.ColorCode(style)
		}
	}

	return survey.WithStdio(in, out, io.ErrOut), nil
}

var errOrgSlugRequired = NonInteractiveError("org slug must be specified when not running interactively")

// Org returns the Organization the user has passed in via flag or prompts the
// user for one.
func Org(ctx context.Context) (*fly.Organization, error) {
	client := flyutil.ClientFromContext(ctx)

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

func SelectOrg(ctx context.Context, orgs []fly.Organization) (org *fly.Organization, err error) {
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

var (
	errRegionCodeRequired  = NonInteractiveError("region code must be specified when not running interactively")
	errRegionCodesRequired = NonInteractiveError("regions codes must be specified in a comma-separated when not running interactively")
)

type RegionInfo struct {
	Regions       []fly.Region
	DefaultRegion *fly.Region
}

var (
	regionsOnce   sync.Once
	regionsFuture *future.Future[RegionInfo]
)

// Fetches all Fly regions and app's default region.
// Only the first call to this function will issue an HTTP request using ctx.
// Subsequent calls will return the same future as the first.
func PlatformRegions(ctx context.Context) *future.Future[RegionInfo] {
	regionsOnce.Do(func() {
		regionsFuture = future.Spawn(func() (RegionInfo, error) {
			client := flyutil.ClientFromContext(ctx)
			regions, defaultRegion, err := client.PlatformRegions(ctx)
			regionInfo := RegionInfo{
				Regions:       regions,
				DefaultRegion: defaultRegion,
			}
			return regionInfo, err
		})
	})

	return regionsFuture
}

func sortedRegions(ctx context.Context, excludedRegionCodes []string) ([]fly.Region, *fly.Region, error) {
	regionInfo, err := PlatformRegions(ctx).Get()
	if err != nil {
		return nil, nil, err
	}

	regions := regionInfo.Regions
	defaultRegion := regionInfo.DefaultRegion

	if len(excludedRegionCodes) > 0 {
		regions = lo.Filter(regions, func(r fly.Region, _ int) bool {
			return !lo.Contains(excludedRegionCodes, r.Code)
		})
	}

	sort.RegionsByNameAndCode(regions)

	return regions, defaultRegion, err
}

// Region returns the region the user has passed in via flag or prompts the
// user for one.
func MultiRegion(ctx context.Context, msg string, splitPaid bool, currentRegions []string, excludedRegionCodes []string, flagName string) (*[]fly.Region, error) {
	regions, _, err := sortedRegions(ctx, excludedRegionCodes)
	paidOnly := []fly.Region{}
	availableRegions := []fly.Region{}
	if err != nil {
		return nil, err
	}

	if splitPaid {
		for _, region := range regions {
			if region.RequiresPaidPlan {
				paidOnly = append(paidOnly, region)
			} else {
				availableRegions = append(availableRegions, region)
			}
		}

		paidOnly = sortAndCleanRegions(paidOnly, excludedRegionCodes)
		regions = sortAndCleanRegions(availableRegions, excludedRegionCodes)
	}

	regionsList := flag.GetString(ctx, flagName)
	regionCodes := strings.Split(regionsList, ",")

	switch {
	case regionsList != "":

		regions = lo.Filter(regions, func(region fly.Region, _ int) bool {
			return lo.ContainsBy(regionCodes, func(regionCode string) bool {
				return regionCode == region.Code
			})
		})
		return &regions, nil
	default:

		switch regions, err := MultiSelectRegion(ctx, msg, paidOnly, regions, currentRegions); {
		case err == nil:
			return &regions, nil
		case IsNonInteractive(err):
			return nil, errRegionCodesRequired
		default:
			return nil, err
		}
	}
}

// Region returns the region the user has passed in via flag or prompts the
// user for one.
func Region(ctx context.Context, splitPaid bool, params RegionParams) (*fly.Region, error) {
	regions, defaultRegion, err := sortedRegions(ctx, params.ExcludedRegionCodes)
	paidOnly := []fly.Region{}
	availableRegions := []fly.Region{}
	if err != nil {
		return nil, err
	}

	if splitPaid {
		for _, region := range regions {
			if region.RequiresPaidPlan {
				paidOnly = append(paidOnly, region)
			} else {
				availableRegions = append(availableRegions, region)
			}
		}

		paidOnly = sortAndCleanRegions(paidOnly, params.ExcludedRegionCodes)
		regions = sortAndCleanRegions(availableRegions, params.ExcludedRegionCodes)
	}

	slug := flag.GetString(ctx, "region")
	if slug == "" {
		slug = config.FromContext(ctx).Region
	}

	switch {
	case slug != "":
		for _, region := range regions {
			if slug == region.Code {
				return &region, nil
			}
		}

		for _, region := range paidOnly {
			if slug == region.Code {
				return nil, fmt.Errorf("region %s requires an organization with a Launch plan or higher. See our plans: https://fly.io/plans", slug)
			}
		}

		return nil, fmt.Errorf("region %s not found", slug)
	default:
		var defaultRegionCode string
		if defaultRegion != nil {
			defaultRegionCode = defaultRegion.Code
		}

		switch region, err := SelectRegion(ctx, params.Message, paidOnly, regions, defaultRegionCode); {
		case err == nil:
			return region, nil
		case IsNonInteractive(err):
			return nil, errRegionCodeRequired
		default:
			return nil, err
		}
	}
}

func sortAndCleanRegions(regions []fly.Region, excludedRegionCodes []string) []fly.Region {
	if len(excludedRegionCodes) > 0 {
		regions = lo.Filter(regions, func(r fly.Region, _ int) bool {
			return !lo.Contains(excludedRegionCodes, r.Code)
		})
	}

	sort.RegionsByNameAndCode(regions)

	return regions
}

func SelectRegion(ctx context.Context, msg string, paid []fly.Region, regions []fly.Region, defaultCode string) (region *fly.Region, err error) {
	var defaultOption string
	var options []string
	if isInteractive(ctx) && len(paid) > 0 {
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.ErrOut, "Some regions require a Launch plan or higher (%s).\nSee https://fly.io/plans to set up a plan.\n\n", strings.Join(lo.Map(paid, func(r fly.Region, _ int) string { return r.Code }), ", "))
	}

	for _, r := range regions {
		option := fmt.Sprintf("%s (%s)", r.Name, r.Code)
		if r.Code == defaultCode {
			defaultOption = option
		}

		options = append(options, option)
	}

	if msg == "" {
		msg = "Select region:"
	}

	var index int
	if err = Select(ctx, &index, msg, defaultOption, options...); err == nil {
		region = &regions[index]
	}

	return
}

func MultiSelectRegion(ctx context.Context, msg string, paid []fly.Region, regions []fly.Region, currentRegions []string) (selectedRegions []fly.Region, err error) {
	var options []string

	var currentIndices []int
	var indices []int

	if isInteractive(ctx) && len(paid) > 0 {
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.ErrOut, "Some regions require a Launch plan or higher (%s).\nSee https://fly.io/plans to set up a plan.\n\n", strings.Join(lo.Map(paid, func(r fly.Region, _ int) string { return r.Code }), ", "))
	}

	for i, r := range regions {
		if lo.Contains(currentRegions, r.Code) {
			currentIndices = append(currentIndices, i)
		}
		option := fmt.Sprintf("%s (%s)", r.Name, r.Code)
		options = append(options, option)
	}

	if msg == "" {
		msg = "Select regions:"
	}

	if err = MultiSelect(ctx, &indices, msg, currentIndices, options...); err == nil {
		for _, index := range indices {
			selectedRegions = append(selectedRegions, regions[index])
		}
	}
	return
}

func SelectVMSize(ctx context.Context, vmSizes []fly.VMSize) (vmSize *fly.VMSize, err error) {
	options := []string{}

	for _, vmSize := range vmSizes {
		if vmSize.MemoryMB < 1024 {
			options = append(options, fmt.Sprintf("%s - CPU Kind: %s, vCPUs: %0.0f Memory: %dMB", vmSize.Name, strings.Title(vmSize.CPUClass), vmSize.CPUCores, vmSize.MemoryMB))
		} else {
			options = append(options, fmt.Sprintf("%s - CPU Kind: %s, vCPUs: %0.0f, Memory: %0.0fGB", vmSize.Name, strings.Title(vmSize.CPUClass), vmSize.CPUCores, vmSize.MemoryGB))
		}
	}

	var index int

	if err := Select(ctx, &index, "Select VM size:", "", options...); err != nil {
		return nil, err
	}
	return &vmSizes[index], nil
}

func SelectAppName(ctx context.Context) (name string, err error) {
	const msg = "Choose an app name (leave blank to generate one):"
	return SelectAppNameWithMsg(ctx, msg)
}

func SelectAppNameWithMsg(ctx context.Context, msg string) (name string, err error) {
	if err = String(ctx, &name, msg, "", false); IsNonInteractive(err) {
		err = NonInteractiveError("name argument or flag must be specified when not running interactively")
	}
	return
}
