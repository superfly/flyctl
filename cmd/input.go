package cmd

import (
	"fmt"
	"path"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
)

func isInterrupt(err error) bool {
	return err != nil && err.Error() == "interrupt"
}

func confirm(message string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: message,
	}
	survey.AskOne(prompt, &confirm)

	return confirm
}

func selectOrganization(client *api.Client, slug string) (*api.Organization, error) {
	orgs, err := client.GetOrganizations()
	if err != nil {
		return nil, err
	}

	if slug != "" {
		for _, org := range orgs {
			if org.Slug == slug {
				return &org, nil
			}
		}

		return nil, fmt.Errorf(`organization "%s" not found`, slug)
	}

	sort.Slice(orgs[:], func(i, j int) bool { return orgs[i].Type < orgs[j].Type })

	options := []string{}

	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	selectedOrg := 0
	prompt := &survey.Select{
		Message:  "Select organization:",
		Options:  options,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedOrg); err != nil {
		return nil, err
	}

	return &orgs[selectedOrg], nil
}

func selectZone(client *api.Client, orgslug string, slug string) (string, error) {
	zones, err := client.GetDNSZones(orgslug)
	if err != nil {
		return "", err
	}

	sort.Slice(zones[:], func(i, j int) bool { return (*zones[i]).Domain < (*zones[j]).Domain })

	options := []string{}

	for _, zone := range zones {
		options = append(options, fmt.Sprintf("%s", zone.Domain))
	}

	selectedOrg := 0
	prompt := &survey.Select{
		Message:  "Select zone:",
		Options:  options,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedOrg); err != nil {
		return "", err
	}

	return (*zones[selectedOrg]).Domain, nil
}

type suggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

var suggestedBuilders = []suggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder",
		DefaultDescription: "GCP Builder for all runtimes",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:base",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, & .NET Core",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:full-cf",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:tiny",
		DefaultDescription: "Tiny base image (bionic build image, distroless run image) with buildpacks for Golang",
	},
	{
		Vendor:             "Fly",
		Image:              "flyio/builder",
		DefaultDescription: "Fly's own Buildpack - currently supporting Deno",
	},
}

var builtins []builtinsupport.Builtin

func selectBuildtype(commandContext *cmdctx.CmdContext) (string, bool, error) {

	dockerfileExists := helpers.FileExists(path.Join(commandContext.WorkingDir, "Dockerfile"))

	dockerfileentry := -1

	builders := []string{}

	builtins = builtinsupport.GetBuiltins()

	for _, b := range builtins {
		builders = append(builders, fmt.Sprintf("%s\n    %s", b.Name, helpers.WrapString(b.Description, 60, 4)))
	}

	if dockerfileExists {
		builders = append(builders, fmt.Sprintf("%s\n    (%s)", "Dockerfile", "Use the existing Dockerfile"))
		dockerfileentry = len(builders) - 1
	}

	for _, b := range suggestedBuilders {
		builders = append(builders, fmt.Sprintf("%s\n    %s", b.Image, helpers.WrapString(b.DefaultDescription, 60, 4)))
	}

	selectedBuilder := 0

	prompt := &survey.Select{
		Message:  "Select builder:",
		Options:  builders,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedBuilder); err != nil {
		return "", false, err
	}

	offset := 0 // offset of the builder names

	if dockerfileentry != -1 {
		offset = offset + 1
		if selectedBuilder == dockerfileentry {
			return "Dockerfile", false, nil
		}
	}

	if selectedBuilder < len(builtins) {
		// Selected a built in
		return builtins[selectedBuilder].Name, true, nil
	}

	offset = offset + len(builtins)

	return suggestedBuilders[selectedBuilder-offset].Image, false, nil
}
