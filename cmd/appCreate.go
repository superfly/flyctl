package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

func newAppCreateCommand() *cobra.Command {
	create := &appCreateCommand{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "create a new app",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return create.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return create.Run(args)
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&create.appName, "app", "a", "", `the app name to use`)
	fs.StringVarP(&create.runtime, "runtime", "r", "", `the runtime to use ("container" or "javascript")`)
	fs.StringVarP(&create.orgSlug, "org", "o", "", "the organization that will own the app")

	return cmd
}

type appCreateCommand struct {
	client  *api.Client
	appName string
	runtime string
	orgID   string
	orgSlug string
}

func (cmd *appCreateCommand) Init() error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}
	cmd.client = client

	return nil
}

func (cmd *appCreateCommand) Run(args []string) error {
	if err := cmd.setAppName(); err != nil {
		return fmt.Errorf("Error setting app name: %s", err)
	}

	if err := cmd.setRuntime(); err != nil {
		return fmt.Errorf("Error setting runtime: %s", err)
	}

	if err := cmd.setTargetOrg(); err != nil {
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

	req := cmd.client.NewRequest(q)

	req.Var("input", map[string]string{
		"organizationId": cmd.orgID,
		"runtime":        cmd.runtime,
		"name":           cmd.appName,
	})

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	newApp := data.CreateApp.App

	fmt.Println("Created new app", newApp.Name)

	manifest := &flyctl.Manifest{
		AppName: newApp.Name,
	}

	cwd, _ := os.Getwd()
	manifestPath := path.Join(cwd, "fly.toml")

	if !helpers.FileExists(manifestPath) && manifest.Save(manifestPath) == nil {
		fmt.Println("Created fly.toml")
	} else {
		fmt.Printf(
			"Configure flyctl by placing the following in a fly.toml file:\n\n%s\n\n",
			manifest.RenderToString(),
		)
	}

	return nil
}

func (cmd *appCreateCommand) setAppName() error {
	if cmd.appName == "" {
		prompt := promptui.Prompt{
			Label: "App Name (leave blank to use an auto-generated name)",
		}

		name, err := prompt.Run()
		if err != nil {
			return err
		}
		cmd.appName = name
	}

	return nil
}

func (cmd *appCreateCommand) setTargetOrg() error {
	q := `
		{
			organizations {
				nodes {
					id
					slug
					name
					type
				}
			}
		}
	`

	req := cmd.client.NewRequest(q)

	data, err := cmd.client.Run(req)
	if err != nil {
		return err
	}

	organizations := data.Organizations.Nodes

	var targetOrgID string

	if targetOrgSlug := viper.GetString("org"); targetOrgSlug != "" {
		for _, org := range organizations {
			if org.Slug == targetOrgSlug {
				targetOrgID = org.ID
				break
			}
		}

		if targetOrgSlug == "" {
			return fmt.Errorf(`orgnaization "%s" not found`, targetOrgSlug)
		}
	}

	if targetOrgID == "" {
		prompt := promptui.Select{
			Label: "Select Organization",
			Items: organizations,
			Size:  16,
			Templates: &promptui.SelectTemplates{
				Active:   "❯ {{ .Name }} ({{ .Slug }})",
				Inactive: "  {{ .Name }} ({{ .Slug }})",
			},
		}

		i, _, err := prompt.Run()
		if err != nil {
			return err
		}

		targetOrgID = organizations[i].ID
	}

	if targetOrgID == "" {
		return fmt.Errorf("No organization selected")
	}

	cmd.orgID = targetOrgID

	return nil
}

func (cmd *appCreateCommand) setRuntime() error {
	if cmd.runtime == "" {
		prompt := promptui.Select{
			Label: "Select Runtime",
			Items: []string{"Container", "JavaScript"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			return err
		}

		cmd.runtime = strings.ToLower(result)
	}

	switch cmd.runtime {
	case "container":
		cmd.runtime = "FIRECRACKER"
	case "javascript":
		cmd.runtime = "NODEPROXY"
	default:
		return fmt.Errorf("Invalid runtime: %s", cmd.runtime)
	}

	return nil
}

func translateRuntime(input string) string {
	switch input {
	case "container":
		return "FIRECRACKER"
	case "javascript":
		return "NODEPROXY"
	}
	return ""
}
