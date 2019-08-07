package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func newAppCreateCommand() *Command {
	cmd := BuildCommand(runAppCreate, "create", "create a new app", os.Stdout, true)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the app name to use",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "runtime",
		Description: `the runtime to use ("container" or "javascript")`,
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})

	return cmd
}

type appCreateCommand struct {
	client  *api.Client
	appName string
	runtime string
	orgID   string
	orgSlug string
}

func runAppCreate(ctx *CmdContext) error {
	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := promptui.Prompt{
			Label: "App Name (leave blank to use an auto-generated name)",
		}
		name, _ = prompt.Run()
	}

	runtime, _ := ctx.Config.GetString("runtime")
	if runtime == "" {
		prompt := promptui.Select{
			Label: "Select Runtime",
			Items: []string{"Container", "JavaScript"},
		}
		_, runtime, _ = prompt.Run()
	}

	switch strings.ToLower(runtime) {
	case "container":
		runtime = "FIRECRACKER"
	case "javascript":
		runtime = "NODEPROXY"
	default:
		return fmt.Errorf("Invalid runtime: %s", runtime)
	}

	org, err := setTargetOrg(ctx)
	if err != nil {
		return fmt.Errorf("Error setting organization: %s", err)
	}

	q := `
	  mutation($input: CreateAppInput!) {
			createApp(input: $input) {
				app {
					id
					name
					runtime
					appUrl
				}
			}
		}
	`

	req := ctx.FlyClient.NewRequest(q)

	req.Var("input", map[string]string{
		"organizationId": org,
		"runtime":        runtime,
		"name":           name,
	})

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	newApp := data.CreateApp.App

	fmt.Println("Created new app", newApp.Name)

	p := flyctl.NewProject(".")
	p.SetAppName(newApp.Name)

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
