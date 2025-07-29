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
				Required:    false,
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

	{
		ToolName:        "fly-machine-cordon",
		ToolDescription: "Deactivate all services on a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to cordon",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "cordon"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			cmdArgs = append(cmdArgs, "--verbose")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-create",
		ToolDescription: "Create, but don’t start, a machine",
		ToolArgs: map[string]FlyArg{
			// missing: build-depot, build-nixpacks, dockerfile, file-literal, file-local, file-secret,
			// kernel-arg, machine-config, org
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"autostart": {
				Description: "Automatically start a stopped Machine when a network request is received",
				Required:    false,
				Type:        "boolean",
				Default:     "true",
			},
			"autostop": {
				Description: "Automatically stop a Machine when there are no network requests for it",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"off", "stop", "suspend"},
				Default:     "off",
			},
			"entrypoint": {
				Description: "The command to override the Docker ENTRYPOINT",
				Required:    false,
				Type:        "string",
			},
			"env": {
				Description: "Set of environment variables in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"host-dedication-id": {
				Description: "The dedication id of the reserved hosts for your organization (if any)",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "Machine ID, if previously known",
				Required:    false,
				Type:        "string",
			},
			"image": {
				Description: "The image to use for the new machine",
				Required:    true,
				Type:        "string",
			},
			"metadata": {
				Description: "Set of metadata in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"name": {
				Description: "Name of the new machine. Will be generated if omitted.",
				Required:    false,
				Type:        "string",
			},
			"port": {
				Description: "The external ports and handlers for services, in the format: port[:machinePort][/protocol[:handler[:handler...]]])",
				Required:    false,
				Type:        "array",
			},
			"region": {
				Description: "Region to create the new machine in",
				Required:    false,
				Type:        "string",
			},
			"restart": {
				Description: "Restart policy for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"no", "always", "on-fail"},
			},
			"rm": {
				Description: "Automatically remove the Machine when it exits",
				Required:    false,
				Type:        "boolean",
			},
			"schedule": {
				Description: "Schedule for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"hourly", "daily", "monthly"},
			},
			"skip-dns-registration": {
				Description: "Skip DNS registration for the new machine",
				Required:    false,
				Type:        "boolean",
			},
			"standby-for": {
				Description: "For Machines without services, a comma separated list of Machine IDs to act as standby for.",
				Required:    false,
				Type:        "array",
			},
			"use-zstd": {
				Description: "Use zstd compression for the image",
				Required:    false,
				Type:        "boolean",
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
			"volume": {
				Description: "Volume to mount, in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
				Required:    false,
				Type:        "array",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "create"}

			if image, ok := args["image"]; ok {
				cmdArgs = append(cmdArgs, image)
			} else {
				return nil, fmt.Errorf("missing required argument: image")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if autostart, ok := args["autostart"]; ok {
				value, err := strconv.ParseBool(autostart)
				if err != nil {
					return nil, fmt.Errorf("invalid value for autostart: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--autostart")
				}
			}

			if autostop, ok := args["autostop"]; ok {
				cmdArgs = append(cmdArgs, "--autostop", autostop)
			}

			if entrypoint, ok := args["entrypoint"]; ok {
				cmdArgs = append(cmdArgs, "--entrypoint", entrypoint)
			}

			if env, ok := args["env"]; ok {
				cmdArgs = append(cmdArgs, "--env", env)
			}

			if hostDedicationID, ok := args["host-dedication-id"]; ok {
				cmdArgs = append(cmdArgs, "--host-dedication-id", hostDedicationID)
			}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, "--id", id)
			}

			if metadata, ok := args["metadata"]; ok {
				cmdArgs = append(cmdArgs, "--metadata", metadata)
			}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, "--name", name)
			}

			if port, ok := args["port"]; ok {
				cmdArgs = append(cmdArgs, "--port", port)
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			if restart, ok := args["restart"]; ok {
				cmdArgs = append(cmdArgs, "--restart", restart)
			}

			if rm, ok := args["rm"]; ok {
				value, err := strconv.ParseBool(rm)
				if err != nil {
					return nil, fmt.Errorf("invalid value for rm: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--rm")
				}
			}

			if schedule, ok := args["schedule"]; ok {
				cmdArgs = append(cmdArgs, "--schedule", schedule)
			}

			if skipDnsRegistration, ok := args["skip-dns-registration"]; ok {
				value, err := strconv.ParseBool(skipDnsRegistration)
				if err != nil {
					return nil, fmt.Errorf("invalid value for skip-dns-registration: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--skip-dns-registration")
				}
			}

			if standbyFor, ok := args["standby-for"]; ok {
				cmdArgs = append(cmdArgs, "--standby-for", standbyFor)
			}

			if useZstd, ok := args["use-zstd"]; ok {
				value, err := strconv.ParseBool(useZstd)
				if err != nil {
					return nil, fmt.Errorf("invalid value for use-zstd: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--use-zstd")
				}
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

			if volume, ok := args["volume"]; ok {
				cmdArgs = append(cmdArgs, "--volume", volume)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-destroy",
		ToolDescription: "Destroy one or more Fly machines. This command requires a machine to be in a stopped or suspended state unless the force flag is used.",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to destroy",
				Required:    true,
				Type:        "string",
			},
			"force": {
				Description: "Force destroy the machine, even if it is running",
				Required:    false,
				Type:        "boolean",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "destroy"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if force, ok := args["force"]; ok {
				value, err := strconv.ParseBool(force)
				if err != nil {
					return nil, fmt.Errorf("invalid value for force: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--force")
				}
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-egress-ip-allocate",
		ToolDescription: "Allocate a pair of static egress IPv4 and IPv6 for a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to allocate egress IP for",
				Required:    true,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "egress-ip", "allocate"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			cmdArgs = append(cmdArgs, "--yes")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-egress-ip-list",
		ToolDescription: "List all static egress IPv4 and IPv6 for a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to list egress IP for",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "egress-ip", "list"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			cmdArgs = append(cmdArgs, "--verbose")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-egress-ip-release",
		ToolDescription: "Release a pair of static egress IPv4 and IPv6 for a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to release egress IP for",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "egress-ip", "release"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			cmdArgs = append(cmdArgs, "--yes")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-exec",
		ToolDescription: "Run a command on a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to run the command on",
				Required:    true,
				Type:        "string",
			},
			"command": {
				Description: "Command to run on the machine",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "exec"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if command, ok := args["command"]; ok {
				cmdArgs = append(cmdArgs, command)
			} else {
				return nil, fmt.Errorf("missing required argument: command")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-leases-clear",
		ToolDescription: "Clear the leases for a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to clear leases for",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "leases", "clear"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-kill",
		ToolDescription: "Kill (SIGKILL) a Fly machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to kill",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "kill"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-leases-view",
		ToolDescription: "View machine leases",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to list leases for",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "leases", "view"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-list",
		ToolDescription: "List all machines for a Fly app",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "list"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			cmdArgs = append(cmdArgs, "--json")

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-restart",
		ToolDescription: "Restart a Fly machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to restart",
				Required:    true,
				Type:        "string",
			},
			"force": {
				Description: "Force stop if it is running",
				Required:    false,
				Type:        "boolean",
			},
			"signal": {
				Description: "Signal to send to the machine",
				Required:    false,
				Type:        "string",
			},
			"skip-health-checks": {
				Description: "Skip health checks during the restart",
				Required:    false,
				Type:        "boolean",
			},
			"time": {
				Description: "Seconds to wait before killing the machine",
				Required:    false,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "restart"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if force, ok := args["force"]; ok {
				value, err := strconv.ParseBool(force)
				if err != nil {
					return nil, fmt.Errorf("invalid value for force: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--force")
				}
			}

			if signal, ok := args["signal"]; ok {
				cmdArgs = append(cmdArgs, "--signal", signal)
			}

			if skipHealthChecks, ok := args["skip-health-checks"]; ok {
				value, err := strconv.ParseBool(skipHealthChecks)
				if err != nil {
					return nil, fmt.Errorf("invalid value for skip-health-checks: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--skip-health-checks")
				}
			}

			if timeStr, ok := args["time"]; ok {
				cmdArgs = append(cmdArgs, "--time", timeStr)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-run",
		ToolDescription: "Run a machine",
		ToolArgs: map[string]FlyArg{
			// missing: build-depot, build-nixpacks, dockerfile, file-literal, file-local, file-secret,
			// kernel-arg, machine-config, org, wg
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"autostart": {
				Description: "Automatically start a stopped Machine when a network request is received",
				Required:    false,
				Type:        "boolean",
				Default:     "true",
			},
			"autostop": {
				Description: "Automatically stop a Machine when there are no network requests for it",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"off", "stop", "suspend"},
				Default:     "off",
			},
			"command": {
				Description: "Command to run on the machine",
				Required:    false,
				Type:        "string",
			},
			"entrypoint": {
				Description: "The command to override the Docker ENTRYPOINT",
				Required:    false,
				Type:        "string",
			},
			"env": {
				Description: "Set of environment variables in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"host-dedication-id": {
				Description: "The dedication id of the reserved hosts for your organization (if any)",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "Machine ID, if previously known",
				Required:    false,
				Type:        "string",
			},
			"image": {
				Description: "The image to use for the new machine",
				Required:    true,
				Type:        "string",
			},
			"metadata": {
				Description: "Set of metadata in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"name": {
				Description: "Name of the new machine. Will be generated if omitted.",
				Required:    false,
				Type:        "string",
			},
			"port": {
				Description: "The external ports and handlers for services, in the format: port[:machinePort][/protocol[:handler[:handler...]]])",
				Required:    false,
				Type:        "array",
			},
			"region": {
				Description: "Region to create the new machine in",
				Required:    false,
				Type:        "string",
			},
			"restart": {
				Description: "Restart policy for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"no", "always", "on-fail"},
			},
			"rm": {
				Description: "Automatically remove the Machine when it exits",
				Required:    false,
				Type:        "boolean",
			},
			"schedule": {
				Description: "Schedule for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"hourly", "daily", "monthly"},
			},
			"skip-dns-registration": {
				Description: "Skip DNS registration for the new machine",
				Required:    false,
				Type:        "boolean",
			},
			"standby-for": {
				Description: "For Machines without services, a comma separated list of Machine IDs to act as standby for.",
				Required:    false,
				Type:        "array",
			},
			"use-zstd": {
				Description: "Use zstd compression for the image",
				Required:    false,
				Type:        "boolean",
			},
			"user": {
				Description: "User to run the command as",
				Required:    false,
				Type:        "string",
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
			"volume": {
				Description: "Volume to mount, in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
				Required:    false,
				Type:        "array",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "run"}

			if image, ok := args["image"]; ok {
				cmdArgs = append(cmdArgs, image)
			} else {
				return nil, fmt.Errorf("missing required argument: image")
			}

			if command, ok := args["command"]; ok {
				cmdArgs = append(cmdArgs, command)
			} else {
				return nil, fmt.Errorf("missing required argument: command")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if autostart, ok := args["autostart"]; ok {
				value, err := strconv.ParseBool(autostart)
				if err != nil {
					return nil, fmt.Errorf("invalid value for autostart: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--autostart")
				}
			}

			if autostop, ok := args["autostop"]; ok {
				cmdArgs = append(cmdArgs, "--autostop", autostop)
			}

			if entrypoint, ok := args["entrypoint"]; ok {
				cmdArgs = append(cmdArgs, "--entrypoint", entrypoint)
			}

			if env, ok := args["env"]; ok {
				cmdArgs = append(cmdArgs, "--env", env)
			}

			if hostDedicationID, ok := args["host-dedication-id"]; ok {
				cmdArgs = append(cmdArgs, "--host-dedication-id", hostDedicationID)
			}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, "--id", id)
			}

			if metadata, ok := args["metadata"]; ok {
				cmdArgs = append(cmdArgs, "--metadata", metadata)
			}

			if name, ok := args["name"]; ok {
				cmdArgs = append(cmdArgs, "--name", name)
			}

			if port, ok := args["port"]; ok {
				cmdArgs = append(cmdArgs, "--port", port)
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			if restart, ok := args["restart"]; ok {
				cmdArgs = append(cmdArgs, "--restart", restart)
			}

			if rm, ok := args["rm"]; ok {
				value, err := strconv.ParseBool(rm)
				if err != nil {
					return nil, fmt.Errorf("invalid value for rm: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--rm")
				}
			}

			if schedule, ok := args["schedule"]; ok {
				cmdArgs = append(cmdArgs, "--schedule", schedule)
			}

			if skipDnsRegistration, ok := args["skip-dns-registration"]; ok {
				value, err := strconv.ParseBool(skipDnsRegistration)
				if err != nil {
					return nil, fmt.Errorf("invalid value for skip-dns-registration: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--skip-dns-registration")
				}
			}

			if standbyFor, ok := args["standby-for"]; ok {
				cmdArgs = append(cmdArgs, "--standby-for", standbyFor)
			}

			if useZstd, ok := args["use-zstd"]; ok {
				value, err := strconv.ParseBool(useZstd)
				if err != nil {
					return nil, fmt.Errorf("invalid value for use-zstd: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--use-zstd")
				}
			}

			if user, ok := args["user"]; ok {
				cmdArgs = append(cmdArgs, "--user", user)
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

			if volume, ok := args["volume"]; ok {
				cmdArgs = append(cmdArgs, "--volume", volume)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-start",
		ToolDescription: "Start a Fly machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to start",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "start"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-status",
		ToolDescription: "Show current status of a running machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to show status for",
				Required:    true,
				Type:        "string",
			},
			"display-config": {
				Description: "Display the machine config",
				Required:    false,
				Type:        "boolean",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "status"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if displayConfig, ok := args["display-config"]; ok {
				value, err := strconv.ParseBool(displayConfig)
				if err != nil {
					return nil, fmt.Errorf("invalid value for display-config: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--display-config")
				}
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-stop",
		ToolDescription: "Stop a Fly machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to stop",
				Required:    true,
				Type:        "string",
			},
			"signal": {
				Description: "Signal to send to the machine",
				Required:    false,
				Type:        "string",
			},
			"timeout": {
				Description: "Seconds to wait before killing the machine",
				Required:    false,
				Type:        "number",
			},
			"wait-timeout": {
				Description: "Seconds to wait for the machine to stop",
				Required:    false,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "stop"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if signal, ok := args["signal"]; ok {
				cmdArgs = append(cmdArgs, "--signal", signal)
			}

			if timeoutStr, ok := args["timeout"]; ok {
				cmdArgs = append(cmdArgs, "--timeout", timeoutStr)
			}

			if waitTimeoutStr, ok := args["wait-timeout"]; ok {
				cmdArgs = append(cmdArgs, "--wait-timeout", waitTimeoutStr)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-suspend",
		ToolDescription: "Suspend a Fly machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to suspend",
				Required:    true,
				Type:        "string",
			},
			"wait-timeout": {
				Description: "Seconds to wait for the machine to suspend",
				Required:    false,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "suspend"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if waitTimeoutStr, ok := args["wait-timeout"]; ok {
				cmdArgs = append(cmdArgs, "--wait-timeout", waitTimeoutStr)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-upcordon",
		ToolDescription: "Reactivate all services on a machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    false,
				Type:        "string",
			},
			"id": {
				Description: "ID of the machine to upcordon",
				Required:    true,
				Type:        "string",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "upcordon"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			return cmdArgs, nil
		},
	},

	{
		ToolName:        "fly-machine-update",
		ToolDescription: "Update a machine",
		ToolArgs: map[string]FlyArg{
			// missing: build-depot, build-nixpacks, container, dockerfile, file-literal, file-local, file-secret,
			// kernel-arg, machine-config
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"id": {
				Description: "Machine ID",
				Required:    true,
				Type:        "string",
			},
			"autostart": {
				Description: "Automatically start a stopped Machine when a network request is received",
				Required:    false,
				Type:        "boolean",
				Default:     "true",
			},
			"autostop": {
				Description: "Automatically stop a Machine when there are no network requests for it",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"off", "stop", "suspend"},
				Default:     "off",
			},
			"command": {
				Description: "Command to run on the machine",
				Required:    false,
				Type:        "string",
			},
			"entrypoint": {
				Description: "The command to override the Docker ENTRYPOINT",
				Required:    false,
				Type:        "string",
			},
			"env": {
				Description: "Set of environment variables in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"host-dedication-id": {
				Description: "The dedication id of the reserved hosts for your organization (if any)",
				Required:    false,
				Type:        "string",
			},
			"image": {
				Description: "The image to use for the new machine",
				Required:    false,
				Type:        "string",
			},
			"metadata": {
				Description: "Set of metadata in the form of NAME=VALUE pairs.",
				Required:    false,
				Type:        "array",
			},
			"port": {
				Description: "The external ports and handlers for services, in the format: port[:machinePort][/protocol[:handler[:handler...]]])",
				Required:    false,
				Type:        "array",
			},
			"restart": {
				Description: "Restart policy for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"no", "always", "on-fail"},
			},
			"schedule": {
				Description: "Schedule for the new machine",
				Required:    false,
				Type:        "enum",
				Enum:        []string{"hourly", "daily", "monthly"},
			},
			"skip-dns-registration": {
				Description: "Skip DNS registration for the new machine",
				Required:    false,
				Type:        "boolean",
			},
			"standby-for": {
				Description: "For Machines without services, a comma separated list of Machine IDs to act as standby for.",
				Required:    false,
				Type:        "array",
			},
			"use-zstd": {
				Description: "Use zstd compression for the image",
				Required:    false,
				Type:        "boolean",
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
			"wait-timeout": {
				Description: "Seconds to wait for the machine to update",
				Required:    false,
				Type:        "number",
			},
		},

		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"machine", "create"}

			if id, ok := args["id"]; ok {
				cmdArgs = append(cmdArgs, id)
			} else {
				return nil, fmt.Errorf("missing required argument: id")
			}

			if image, ok := args["image"]; ok {
				cmdArgs = append(cmdArgs, "--image", image)
			}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			} else {
				return nil, fmt.Errorf("missing required argument: app")
			}

			if autostart, ok := args["autostart"]; ok {
				value, err := strconv.ParseBool(autostart)
				if err != nil {
					return nil, fmt.Errorf("invalid value for autostart: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--autostart")
				}
			}

			if autostop, ok := args["autostop"]; ok {
				cmdArgs = append(cmdArgs, "--autostop", autostop)
			}

			if entrypoint, ok := args["entrypoint"]; ok {
				cmdArgs = append(cmdArgs, "--entrypoint", entrypoint)
			}

			if env, ok := args["env"]; ok {
				cmdArgs = append(cmdArgs, "--env", env)
			}

			if hostDedicationID, ok := args["host-dedication-id"]; ok {
				cmdArgs = append(cmdArgs, "--host-dedication-id", hostDedicationID)
			}

			if metadata, ok := args["metadata"]; ok {
				cmdArgs = append(cmdArgs, "--metadata", metadata)
			}

			if port, ok := args["port"]; ok {
				cmdArgs = append(cmdArgs, "--port", port)
			}

			if restart, ok := args["restart"]; ok {
				cmdArgs = append(cmdArgs, "--restart", restart)
			}

			if rm, ok := args["rm"]; ok {
				value, err := strconv.ParseBool(rm)
				if err != nil {
					return nil, fmt.Errorf("invalid value for rm: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--rm")
				}
			}

			if schedule, ok := args["schedule"]; ok {
				cmdArgs = append(cmdArgs, "--schedule", schedule)
			}

			if skipDnsRegistration, ok := args["skip-dns-registration"]; ok {
				value, err := strconv.ParseBool(skipDnsRegistration)
				if err != nil {
					return nil, fmt.Errorf("invalid value for skip-dns-registration: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--skip-dns-registration")
				}
			}

			if standbyFor, ok := args["standby-for"]; ok {
				cmdArgs = append(cmdArgs, "--standby-for", standbyFor)
			}

			if useZstd, ok := args["use-zstd"]; ok {
				value, err := strconv.ParseBool(useZstd)
				if err != nil {
					return nil, fmt.Errorf("invalid value for use-zstd: %v", err)
				} else if value {
					cmdArgs = append(cmdArgs, "--use-zstd")
				}
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

			if waitTimeout, ok := args["wait-timeout"]; ok {
				cmdArgs = append(cmdArgs, "--wait-timeout", waitTimeout)
			}

			return cmdArgs, nil
		},
	},
}
