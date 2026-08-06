package mcpServer

import (
	"fmt"
	"strconv"
)

var VolumeCommands = []FlyCommand{
	{
		ToolName:        "fly-volumes-create",
		ToolDescription: "Create a new volume for an app. Volumes are persistent storage for Fly Machines.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"name": {
				Description: "name of the volume",
				Required:    true,
				Type:        "string",
			},
			"encrypt": {
				Description: "Encrypt the volume",
				Required:    false,
				Type:        "boolean",
				Default:     "true",
			},
			"region": {
				Description: "Region to create the volume in",
				Required:    true,
				Type:        "string",
			},
			"size": {
				Description: "Size of the volume in GB",
				Required:    false,
				Type:        "number",
				Default:     "1",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "create"}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, name)
			} else {
				return nil, fmt.Errorf("name argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			if encrypt, ok := args["encrypt"]; ok {
				encryptBool, err := strconv.ParseBool(encrypt)
				if err != nil {
					return nil, fmt.Errorf("invalid value for encrypt: %v", err)
				} else if !encryptBool {
					cmdArgs = append(cmdArgs, "--no-encryption")
				}
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			} else {
				return nil, fmt.Errorf("region argument is required")
			}

			if size, ok := args["size"]; ok {
				cmdArgs = append(cmdArgs, "--size", size)
			}

			cmdArgs = append(cmdArgs, "--yes", "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-destroy",
		ToolDescription: "Destroy one or more volumes. When you destroy a volume, you permanently delete all its data.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "destroy"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			cmdArgs = append(cmdArgs, "--yes", "--verbose")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-extend",
		ToolDescription: "Extend a volume to a larger size. You can only extend a volume to a larger size.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
			"size": {
				Description: "Size of the volume in Gigabytes",
				Required:    true,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "extend"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			if size, ok := args["size"]; ok {
				cmdArgs = append(cmdArgs, "--size", size)
			} else {
				return nil, fmt.Errorf("size argument is required")
			}

			cmdArgs = append(cmdArgs, "--yes", "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-fork",
		ToolDescription: "Fork the specified volume. Volume forking creates an independent copy of a storage volume for backup, testing, and experimentation without altering the original data.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
			"region": {
				Description: "Region to create the new volume in",
				Required:    false,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "fork"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-list",
		ToolDescription: "List all volumes for an app. Volumes are persistent storage for Fly Machines.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"all": {
				Description: "Show all volumes, including those that in destroyed states",
				Required:    false,
				Type:        "boolean",
				Default:     "false",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "list"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			if all, ok := args["all"]; ok {
				allBool, err := strconv.ParseBool(all)
				if err != nil {
					return nil, fmt.Errorf("invalid value for all: %v", err)
				} else if allBool {
					cmdArgs = append(cmdArgs, "--all")
				}
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-show",
		ToolDescription: "Show details about a volume. Volumes are persistent storage for Fly Machines.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "show"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-snapshots-create",
		ToolDescription: "Create a snapshot of a volume. Snapshots are point-in-time copies of a volume.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "snapshots", "create"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-snapshots-list",
		ToolDescription: "List all snapshots for a volume. Snapshots are point-in-time copies of a volume.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "snapshots", "list"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-volumes-update",
		ToolDescription: "Update a volume. You can activate or deactivate snapshotting, and change the snapshot's retention period.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "id of the volume",
				Required:    true,
				Type:        "string",
			},
			"scheduled-snapshots": {
				Description: "Enable or disable scheduled snapshots",
				Required:    false,
				Type:        "boolean",
			},
			"snapshot-retention": {
				Description: "Retention period for snapshots in days",
				Required:    false,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"volume", "update"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("id argument is required")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("app argument is required")
			}

			if scheduledSnapshots, ok := args["scheduled-snapshots"]; ok {
				scheduledSnapshotsBool, err := strconv.ParseBool(scheduledSnapshots)
				if err != nil {
					return nil, fmt.Errorf("invalid value for scheduled-snapshots: %v", err)
				} else if scheduledSnapshotsBool {
					cmdArgs = append(cmdArgs, "--scheduled-snapshots=true")
				} else {
					cmdArgs = append(cmdArgs, "--scheduled-snapshots=false")
				}
			}

			if snapshotRetention, ok := args["snapshot-retention"]; ok {
				cmdArgs = append(cmdArgs, "--snapshot-retention", snapshotRetention)
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},
}
