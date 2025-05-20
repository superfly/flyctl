package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

// newVolumeCommand creates the 'volume' command for flyctl.
func newVolume() *cobra.Command {
	const (
		short = "[experimental] mount a fly volume"
		long  = short + "\n"
		usage = "volume"
	)

	cmd := command.New(usage, short, long, runVolume)
	cmd.Args = cobra.ExactArgs(0)
	cmd.Hidden = true

	flag.Add(cmd,
		flag.String{
			Name:        "source",
			Description: "Source of the volume",
			Default:     "data",
		},
		flag.String{
			Name:        "destination",
			Description: "Destination path in the container",
			Default:     "/data",
		},
		flag.String{
			Name:        "initial-size",
			Description: "Initial size of the volume",
			Default:     "1GB",
		},
		flag.Int{
			Name:        "auto-extend-size-threshold",
			Description: "Auto extend size threshold percentage",
			Default:     80,
		},
		flag.String{
			Name:        "auto-extend-size-increment",
			Description: "Auto extend size increment",
			Default:     "1GB",
		},
		flag.String{
			Name:        "auto-extend-size-limit",
			Description: "Auto extend size limit",
			Default:     "10GB",
		},
		flag.Int{
			Name:        "snapshot-retention",
			Description: "Snapshot retention period in days",
			Default:     0,
		},
		flag.String{
			Name:        "server",
			Description: "Name to use for the MCP server in the MCP client configuration",
			Default:     "volume",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Add MCP server to the " + name + " client configuration",
			},
		)
	}

	return cmd
}

// runVolume is the command handler for the 'volume' command
func runVolume(ctx context.Context) error {
	volume := flag.GetString(ctx, "source") + ":" + flag.GetString(ctx, "destination")

	options := []string{}

	if initialSize := flag.GetString(ctx, "initial-size"); initialSize != "" {
		options = append(options, "initial_size="+initialSize)
	}
	if autoExtendSizeThreshold := flag.GetInt(ctx, "auto-extend-size-threshold"); autoExtendSizeThreshold != 0 {
		options = append(options, "auto_extend_size_threshold="+strconv.Itoa(autoExtendSizeThreshold))
	}

	if autoExtendSizeIncrement := flag.GetString(ctx, "auto-extend-size-increment"); autoExtendSizeIncrement != "" {
		options = append(options, "auto_extend_size_increment="+autoExtendSizeIncrement)
	}

	if autoExtendSizeLimit := flag.GetString(ctx, "auto-extend-size-limit"); autoExtendSizeLimit != "" {
		options = append(options, "auto_extend_size_limit="+autoExtendSizeLimit)
	}

	if snapshotRetention := flag.GetInt(ctx, "snapshot-retention"); snapshotRetention != 0 {
		options = append(options, "snapshot_retention="+strconv.Itoa(snapshotRetention))
	}

	if len(options) > 0 {
		volume += ":" + strings.Join(options, ",")
	}

	args := []string{
		"mcp",
		"launch",
		`npx -y @modelcontextprotocol/server-filesystem ` + flag.GetString(ctx, "destination"),
		"--server", flag.GetString(ctx, "server"),
		"--volume", volume,
	}

	// Add the MCP server to the MCP client configurations
	for client := range McpClients {
		if flag.GetBool(ctx, client) {
			log.Debugf("Adding %s to MCP client configuration", client)
			args = append(args, "--"+client)
		}
	}

	for _, config := range flag.GetStringArray(ctx, "config") {
		if config != "" {
			log.Debugf("Adding %s to MCP client configuration", config)
			args = append(args, "--config", config)
		}
	}

	flyctl, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	cmd := exec.Command(flyctl, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to launch MCP volume: %w", err)
	}

	// Check if the command was successful
	if cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("failed to launch MCP volume: %s", cmd.ProcessState.String())
	}

	return nil
}
