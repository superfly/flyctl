package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
)

func NewLaunch() *cobra.Command {
	const (
		short = "[experimental] Launch an MCP stdio program"
		long  = short + `. Options passed after double dashes ("--") will be passed to the MCP program. If user is specified, HTTP authentication will be required.` + "\n"
		usage = "launch"
	)
	cmd := command.New(usage, short, long, runLaunch)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Description: "Name to use for the MCP server in the MCP client configuration",
		},
		flag.String{
			Name:        "user",
			Description: "User to authenticate with",
		},
		flag.String{
			Name:        "password",
			Description: "Password to authenticate with",
		},
		flag.Bool{
			Name:        "bearer-token",
			Description: "Use bearer token for authentication",
			Default:     true,
		},
		flag.Bool{
			Name:        "flycast",
			Description: "Use wireguard and flycast for access",
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

func runLaunch(ctx context.Context) error {
	log := logger.FromContext(ctx)

	flyctl, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	// Parse the command
	command := flag.FirstArg(ctx)
	cmdParts, err := shlex.Split(command)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	} else if len(cmdParts) == 0 {
		return fmt.Errorf("missing command to run")
	}

	// determine the name of the MCP server
	name := flag.GetString(ctx, "name")
	if name == "" {
		name = "fly-mcp"

		ingoreWords := []string{"npx", "uv", "-y", "--yes"}

		for _, w := range cmdParts {
			if !slices.Contains(ingoreWords, w) {
				re := regexp.MustCompile(`[-\w]+`)
				split := re.FindAllString(w, -1)

				if len(split) > 0 {
					name = split[len(split)-1]
					break
				}
			}
		}
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", name)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Debugf("Created temporary directory: %s\n", tempDir)

	if err := os.Chdir(tempDir); err != nil {
		return fmt.Errorf("failed to change to temporary directory: %w", err)
	}

	// Build the Dockerfile
	jsonData, err := json.Marshal(cmdParts)
	if err != nil {
		return fmt.Errorf("failed to marshal command parts to JSON: %w", err)
	}

	dockerfile := []string{
		"FROM flyio/mcp",
		"CMD " + string(jsonData),
	}

	dockerfileContent := strings.Join(dockerfile, "\n") + "\n"

	if err := os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("failed to create Dockerfile: %w", err)
	}

	log.Debug("Created Dockerfile")

	// Run fly launch, but don't deploy
	cmd := exec.Command(flyctl, "launch", "--yes", "--no-deploy")
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run 'fly launch': %w", err)
	}

	log.Debug("Launched fly application")

	args := []string{}

	// Add the MCP server to the MCP client configurations
	for client := range McpClients {
		if flag.GetBool(ctx, client) {
			log.Debugf("Adding %s to MCP client configuration", client)
			args = append(args, "--"+client)
		}
	}

	if len(args) == 0 {
		log.Debug("No MCP client configuration flags provided")
	} else {
		args = append([]string{"mcp", "add"}, args...)
		args = append(args, "--name", name)

		// Run 'fly mcp add ...'
		cmd = exec.Command(flyctl, args...)
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run 'fly mcp add': %w", err)
		}

		log.Debug(strings.Join(args, " "))
	}

	// Deploy to a single machine
	cmd = exec.Command(flyctl, "deploy", "--ha=false")
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run 'fly launch': %w", err)
	}

	log.Debug("Launched fly application")

	log.Debug("Successfully completed MCP server launch and configuration")

	return nil
}
