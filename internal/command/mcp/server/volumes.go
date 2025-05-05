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
}
