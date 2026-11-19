package mcpServer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineCreateAndRunAutostop(t *testing.T) {
	for _, cmd := range MachineCommands {
		if cmd.ToolName != "fly-machine-create" && cmd.ToolName != "fly-machine-run" {
			continue
		}
		args, err := cmd.Builder(map[string]string{"app": "myapp", "image": "nginx", "command": "nginx", "autostop": "off"})
		require.NoError(t, err)
		require.Contains(t, args, "--autostop=off", cmd.ToolName)
		require.NotContains(t, args, "off", cmd.ToolName)
	}
}

func TestMachineCacheDiskArguments(t *testing.T) {
	for _, cmd := range MachineCommands {
		if cmd.ToolName != "fly-machine-create" && cmd.ToolName != "fly-machine-run" && cmd.ToolName != "fly-machine-update" {
			continue
		}
		require.Contains(t, cmd.ToolArgs, "cache-disk-size")
		require.Contains(t, cmd.ToolArgs, "cache-disk-path")
		args, err := cmd.Builder(map[string]string{"app": "myapp", "id": "abc", "image": "nginx", "command": "nginx", "cache-disk-size": "1gb", "cache-disk-path": ""})
		require.NoError(t, err)
		require.Contains(t, args, "--cache-disk-size=1gb")
		require.Contains(t, args, "--cache-disk-path=")
	}
}
