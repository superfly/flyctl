package mcpServer

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"

	"github.com/mark3labs/mcp-go/server"
)

var LogCommands = []FlyCommand{
	{
		ToolName:        "fly-logs",
		ToolDescription: "Get logs for a Fly.io app or specific machine",
		ToolArgs: map[string]FlyArg{
			"app": {
				Description: "Name of the app",
				Required:    true,
				Type:        "string",
			},
			"machine": {
				Description: "Specific machine ID",
				Required:    false,
				Type:        "string",
			},
			"region": {
				Description: "Region to get logs from",
				Required:    false,
				Type:        "string",
			},
		},
		Builder: func(args map[string]string) ([]string, error) {
			cmdArgs := []string{"logs"}

			if app, ok := args["app"]; ok {
				cmdArgs = append(cmdArgs, "-a", app)
			}

			if machine, ok := args["machine"]; ok {
				cmdArgs = append(cmdArgs, "--machine", machine)
			}

			if region, ok := args["region"]; ok {
				cmdArgs = append(cmdArgs, "--region", region)
			}

			if progressToken, ok := args["_meta.progressToken"]; ok {
				cmdArgs = append(cmdArgs, "--progress-token", progressToken)
			} else {
				cmdArgs = append(cmdArgs, "--no-tail")
			}

			return cmdArgs, nil
		},
		Execute: func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
			server := server.ServerFromContext(ctx)

			if len(args) > 2 && args[len(args)-2] == "--progress-token" {
				progressToken := args[len(args)-1]
				args = args[:len(args)-2]

				execCmd := exec.Command(cmd, args...)
				stdout, err := execCmd.StdoutPipe()
				if err != nil {
					return nil, err
				}
				if err := execCmd.Start(); err != nil {
					return nil, err
				}

				var token any
				if err := json.Unmarshal([]byte(progressToken), &token); err != nil {
					return nil, err
				}

				scanner := bufio.NewScanner(stdout)
				i := 1
				for scanner.Scan() {
					err := server.SendNotificationToClient(
						ctx,
						"notifications/progress",
						map[string]any{
							"progress":      i,
							"progressToken": token,
							"message":       scanner.Text(),
						},
					)

					if err != nil {
						return nil, err
					}

					i++
				}
				if err := scanner.Err(); err != nil {
					return nil, err
				}

				execCmd.Wait()
				return nil, nil
			}

			execCmd := exec.Command(cmd, args...)
			return execCmd.CombinedOutput()
		},
	},
}
