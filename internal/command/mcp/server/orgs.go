package mcpServer

import "fmt"

var OrgCommands = []FlyCommand{
	{
		ToolName:        "fly-orgs-create",
		ToolDescription: "Create a new organization. Other users can be invited to join the organization later.",
		ToolArgs: map[string]FlyArg{
			"name": {
				Description: "Name of the organization",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "create"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("missing required argument: name")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-orgs-delete",
		ToolDescription: "Delete an organization. All apps and machines in the organization will be deleted.",
		ToolArgs: map[string]FlyArg{
			"slug": {
				Description: "Slug of the organization to delete",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "delete"}

			if slug, ok := args["slug"]; ok {
				cmdArgs = append(cmdArgs, slug)
			} else {
				return nil, fmt.Errorf("missing required argument: slug")
			}

			cmdArgs = append(cmdArgs, "--yes")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-orgs-invite",
		ToolDescription: "Invite a user, by email, to join organization. The invitation will be sent, and the user will be pending until they respond.",
		ToolArgs: map[string]FlyArg{
			"slug": {
				Description: "Slug of the organization to invite the user to",
				Required:    true,
				Type:        "string",
			},
			"email": {
				Description: "Email address of the user to invite",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "invite"}

			if slug, ok := args["slug"]; ok {
				cmdArgs = append(cmdArgs, slug)
			} else {
				return nil, fmt.Errorf("missing required argument: slug")
			}

			if email, ok := args["email"]; ok {
				cmdArgs = append(cmdArgs, email)
			} else {
				return nil, fmt.Errorf("missing required argument: email")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-orgs-list",
		ToolDescription: "List all organizations the user is a member of.  Keys are names of the organizations, values are slugs.",
		ToolArgs:        map[string]FlyArg{},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "list", "--json"}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-orgs-remove",
		ToolDescription: "Remove a user from an organization. The user will no longer have access to the organization.",
		ToolArgs: map[string]FlyArg{
			"slug": {
				Description: "Slug of the organization to remove the user from",
				Required:    true,
				Type:        "string",
			},
			"email": {
				Description: "Email address of the user to remove",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "remove"}

			if slug, ok := args["slug"]; ok {
				cmdArgs = append(cmdArgs, slug)
			} else {
				return nil, fmt.Errorf("missing required argument: slug")
			}

			if email, ok := args["email"]; ok {
				cmdArgs = append(cmdArgs, email)
			} else {
				return nil, fmt.Errorf("missing required argument: email")
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-orgs-show",
		ToolDescription: "Shows information about an organization. Includes name, slug and type. Summarizes user permissions, DNS zones and associated member. Details full list of members and roles.",
		ToolArgs: map[string]FlyArg{
			"slug": {
				Description: "Slug of the organization to show",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"orgs", "show"}

			if slug, ok := args["slug"]; ok {
				cmdArgs = append(cmdArgs, slug)
			} else {
				return nil, fmt.Errorf("missing required argument: slug")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},
}
