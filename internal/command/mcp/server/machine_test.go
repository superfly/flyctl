package mcpServer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func machineUpdateTool(t *testing.T) FlyCommand {
	for _, cmd := range MachineCommands {
		if cmd.ToolName == "fly-machine-update" {
			return cmd
		}
	}
	t.Fatal("fly-machine-update not found")
	return FlyCommand{}
}

func TestMachineUpdateBuilder(t *testing.T) {
	update := machineUpdateTool(t)

	args, err := update.Builder(map[string]string{"app": "myapp", "id": "148e4d55b13e89", "autostart": "false", "autostop": "suspend", "metadata": "test=1"})
	require.NoError(t, err)
	require.Equal(t, []string{"machine", "update", "148e4d55b13e89", "-a", "myapp", "--autostart=false", "--autostop=suspend", "--metadata", "test=1", "--yes"}, args)
}

func TestMachineUpdateHasNoDefaults(t *testing.T) {
	// The server fills in defaults for missing args, which would make every update reset them.
	for name, arg := range machineUpdateTool(t).ToolArgs {
		require.Empty(t, arg.Default, name)
	}
}
