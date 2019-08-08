package cmd

import (
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
)

func newAppCreateCommand() *Command {
	cmd := BuildCommand(nil, runAppCreate, "create", "create a new app", os.Stdout, true)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the app name to use",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})

	return cmd
}

func runAppCreate(ctx *CmdContext) error {
	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := promptui.Prompt{
			Label: "App Name (leave blank to use an auto-generated name)",
		}
		name, _ = prompt.Run()
	}

	org, err := setTargetOrg(ctx)
	if err != nil {
		return fmt.Errorf("Error setting organization: %s", err)
	}

	app, err := ctx.FlyClient.CreateApp(name, org)
	if err != nil {
		return err
	}

	fmt.Println("New app created")

	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	p := flyctl.NewProject(".")
	p.SetAppName(app.Name)

	if err := p.SafeWriteConfig(); err != nil {
		fmt.Printf(
			"Configure flyctl by placing the following in a fly.toml file:\n\n%s\n\n",
			p.WriteConfigAsString(),
		)
	} else {
		fmt.Println("Created fly.toml")
	}

	return nil
}

func setTargetOrg(ctx *CmdContext) (string, error) {
	orgs, err := ctx.FlyClient.GetOrganizations()
	if err != nil {
		return "", err
	}

	var targetOrgID string

	if targetOrgSlug, _ := ctx.Config.GetString("org"); targetOrgSlug != "" {
		for _, org := range orgs {
			if org.Slug == targetOrgSlug {
				targetOrgID = org.ID
				break
			}
		}

		if targetOrgSlug == "" {
			return "", fmt.Errorf(`orgnaization "%s" not found`, targetOrgSlug)
		}
	}

	if targetOrgID == "" {
		prompt := promptui.Select{
			Label: "Select Organization",
			Items: orgs,
			Size:  16,
			Templates: &promptui.SelectTemplates{
				Active:   "❯ {{ .Name }} ({{ .Slug }})",
				Inactive: "  {{ .Name }} ({{ .Slug }})",
			},
		}

		i, _, _ := prompt.Run()
		targetOrgID = orgs[i].ID
	}

	return targetOrgID, nil
}
