package mcpServer

import (
	"fmt"
	"strconv"
)

var MachineCommands = []FlyCommand{
	{
		ToolName:        "fly-machine-clone",
		ToolDescription: "Clone a Fly Machine. The new Machine will be a copy of the specified Machine. If the original Machine has a volume, then a new empty volume will be created and attached to the new Machine.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to clone",
				Required:    true,
				Type:        "string",
			},
			"attach-volume": {
				Description: "Attach a volume to the new machine",
				Required:    false,
				Type:        "string",
			},
			"clear-auto-destroy": {
				Description: "Disable auto destroy setting on the new Machine",
				Required:    false,
				Type:        "boolean",
			},
			"clear-cmd": {
				Description: "Set empty CMD on the new Machine so it uses default CMD for the image",
				Required:    false,
				Type:        "boolean",
			},
			"from-snapshot": {
				Description: "Clone attached volumes and restore from snapshot, use 'last' for most recent snapshot. The default is an empty volume.",
				Required:    false,
				Type:        "string",
			},
			"host-dedication-id": {
				Description: "The dedication id of the reserved hosts for your organization (if any)",
				Required:    false,
				Type:        "string",
			},
			"name": {
				Description: "Optional name of the new machine",
				Required:    false,
				Type:        "string",
			},
			"override-cmd": {
				Description: "Set CMD on the new Machine to this value",
				Required:    false,
				Type:        "string",
			},
			"region": {
				Description: "Region to create the new machine in",
				Required:    false,
				Type:        "string",
			},
			"standby-for": {
				Description: "Standby for a machine in the same region",
				Required:    false,
				Type:        "array",
			},
			"vm-cpu-kind": {
				Description: "The CPU kind to use for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"shared", "dedicated"},
			},
			"vm-cpus": {
				Description: "The number of CPUs to use for the new machine",
				Required:    false,
				Type:        "number",
			},
			"vm-gpu-kind": {
				Description: "If set, the GPU model to attach",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"a100-pcie-40gb", "a100-sxm4-80gb", "l40s", "a10", "none"},
			},
			"vm-gpus": {
				Description: "The number of GPUs to use for the new machine",
				Required:    false,
				Type:        "number",
			},
			"vm-memory": {
				Description: "The amount of memory (in megabytes) to use for the new machine",
				Required:    false,
				Type:        "number",
			},
			"vm-size": {
				Description: `The VM size to set machines to. See "fly platform vm-sizes" for valid values`,
				Required:    false,
				Type:        "string",
			},
			"volume-requires-unique-zone": {
				Description: "Require volume to be placed in separate hardware zone from existing volumes.",
				Required:    false,
				Type:        "boolean",
				Default:     "true",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "clone"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if attachVolume, ok := args["attach-volume"]; ok {
				cmdArgs = append(cmdArgs, "--attach-volume", attachVolume)
			}

			if clearAutoDestroy, ok := args["clear-auto-destroy"]; ok {
				value, err := strconv.ParseBool(clearAutoDestroy)
				if err != nil {
					return nil, fmt.Errorf("invalid value for clear-auto-destroy: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--clear-auto-destroy")
				}
			}

			if clearCmd, ok := args["clear-cmd"]; ok {
				value, err := strconv.ParseBool(clearCmd)
				if err != nil {
					return nil, fmt.Errorf("invalid value for clear-cmd: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--clear-cmd")
				}
			}

			if fromSnapshot, ok := args["from-snapshot"]; ok {
				cmdArgs = append(cmdArgs, "--from-snapshot", fromSnapshot)
			}

			if hostDedicationID, ok := args["host-dedication-id"]; ok {
				cmdArgs = append(cmdArgs, "--host-dedication-id", hostDedicationID)
			}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, "--name", name)
			}

			if overrideCmd, ok := args["override-cmd"]; ok {
				cmdArgs = append(cmdArgs, "--override-cmd", overrideCmd)
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			if standbyFor, ok := args["standby-for"]; ok {
				cmdArgs = append(cmdArgs, "--standby-for", standbyFor)
			}

			if vmCpuKind, ok := args["vm-cpu-kind"]; ok {
				cmdArgs = append(cmdArgs, "--vm-cpu-kind", vmCpuKind)
			}

			if vmCpus, ok := args["vm-cpus"]; ok {
				cmdArgs = append(cmdArgs, "--vm-cpus", vmCpus)
			}

			if vmGpuKind, ok := args["vm-gpu-kind"]; ok {
				cmdArgs = append(cmdArgs, "--vm-gpu-kind", vmGpuKind)
			}

			if vmGpus, ok := args["vm-gpus"]; ok {
				cmdArgs = append(cmdArgs, "--vm-gpus", vmGpus)
			}

			if vmMemory, ok := args["vm-memory"]; ok {
				cmdArgs = append(cmdArgs, "--vm-memory", vmMemory)
			}

			if vmSize, ok := args["vm-size"]; ok {
				cmdArgs = append(cmdArgs, "--vm-size", vmSize)
			}

			if volumeRequiresUniqueZone, ok := args["volume-requires-unique-zone"]; ok {
				value, err := strconv.ParseBool(volumeRequiresUniqueZone)
				if err != nil {
					return nil, fmt.Errorf("invalid value for volume-requires-unique-zone: %v", err)
				} else if !value {
					cmdArgs = append(cmdArgs, "--volume-requires-unique-zone=false")
				}
			}

			return cmdArgs, nil
		},
	},
}
