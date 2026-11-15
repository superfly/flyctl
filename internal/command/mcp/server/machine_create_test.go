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
