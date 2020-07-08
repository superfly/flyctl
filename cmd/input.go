package cmd

import (
	"fmt"
	"path"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
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

func selectBuildtype(commandContext *cmdctx.CmdContext) (string, error) {

	dockerfileExists := helpers.FileExists(path.Join(commandContext.WorkingDir, "Dockerfile"))

	builders := []string{}

	if dockerfileExists {
		builders = append(builders, fmt.Sprintf("%s (%s)", "Dockerfile", "Use the existing Dockerfile"))
	} else {
		builders = append(builders, fmt.Sprintf("%s (%s)", "Dockerfile", "Create a example Dockerfile"))
	}

	for _, b := range suggestedBuilders {
		builders = append(builders, fmt.Sprintf("%s", b.Image))
	}

	selectedBuilder := 0

	prompt := &survey.Select{
		Message:  "Select builder:",
		Options:  builders,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedBuilder); err != nil {
		return "", err
	}

	if selectedBuilder == 0 {
		return "Dockerfile", nil
	}
	return suggestedBuilders[selectedBuilder-1].Image, nil
}

func confirmFileOverwrite(filename string) bool {
	if helpers.FileExists(filename) {
		return confirm(fmt.Sprintf("Overwrite file '%s'", filename))
	}
	return true
}
