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

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
)

func NewLaunch() *cobra.Command {
	const (
		short = "[experimental] Launch an MCP stdio server"
		long  = short + "\n"
		usage = "launch command"
	)
	cmd := command.New(usage, short, long, runLaunch)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Description: "Suggested name for the app",
		},
		flag.String{
			Name:        "server",
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
		flag.Bool{
			Name:        "inspector",
			Description: "Launch MCP inspector: a developer tool for testing and debugging MCP servers",
			Default:     false,
			Shorthand:   "i",
		},
		flag.StringArray{
			Name:        "config",
			Description: "Path to the MCP client configuration file (can be specified multiple times)",
		},
		flag.String{
			Name:        "auto-stop",
			Description: "Automatically suspend the app after a period of inactivity. Valid values are 'off', 'stop', and 'suspend'",
			Default:     "suspend",
		},
		flag.StringArray{
			Name:        "secret",
			Description: "Set of secrets in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.StringArray{
			Name:        "file-local",
			Description: "Set of files in the form of /path/inside/machine=<local/path> pairs. Can be specified multiple times.",
		},
		flag.StringArray{
			Name:        "file-literal",
			Description: "Set of literals in the form of /path/inside/machine=VALUE pairs where VALUE is the content. Can be specified multiple times.",
		},
		flag.StringArray{
			Name:        "file-secret",
			Description: "Set of secrets in the form of /path/inside/machine=SECRET pairs where SECRET is the name of the secret. Can be specified multiple times.",
		},
		flag.String{
			Name:        "region",
			Shorthand:   "r",
			Description: "The target region. By default, the new volume will be created in the source volume's region.",
		},
		flag.String{
			Name:        "org",
			Description: `The organization that will own the app`,
		},
		flag.StringSlice{
			Name:        "volume",
			Shorthand:   "v",
			Description: "Volume to mount, in the form of <volume_name>:/path/inside/machine[:<options>]",
		},
		flag.String{
			Name:        "image",
			Description: "The image to use for the app",
		},
		flag.StringSlice{
			Name:        "setup",
			Description: "Additional setup commands to run before launching the MCP server",
		},
		flag.VMSizeFlags,
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

	image := flag.GetString(ctx, "image")

	// Parse the command
	command := flag.FirstArg(ctx)
	cmdParts, err := shlex.Split(command)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	} else if len(cmdParts) == 0 && image == "" {
		return fmt.Errorf("missing command or image to run")
	}

	setup := flag.GetStringSlice(ctx, "setup")
	if len(setup) > 0 && image == "" {
		image = "flyio/mcp"
	}

	// extract the entrypoint from the image
	entrypoint := []string{}
	if image != "" {
		ref, err := name.ParseReference(image)
		if err != nil {
			return fmt.Errorf("failed to parse image reference: %w", err)
		}
		img, err := remote.Image(ref)
		if err != nil {
			return fmt.Errorf("failed to find image: %w", err)
		}
		cfg, err := img.ConfigFile()
		if err != nil {
			return fmt.Errorf("failed to get image config: %w", err)
		}
		entrypoint = cfg.Config.Entrypoint

		if len(cmdParts) == 0 {
			cmdParts = cfg.Config.Cmd
		}
	}

	// determine the name of the MCP server
	serverName := flag.GetString(ctx, "server")
	if serverName == "" {
		serverName = flag.GetString(ctx, "name")
	}

	if serverName == "" {
		serverName = "fly-mcp"

		ingoreWords := []string{"npx", "uvx", "-y", "--yes"}

		for _, w := range cmdParts {
			if !slices.Contains(ingoreWords, w) {
				re := regexp.MustCompile(`[-\w]+`)
				split := re.FindAllString(w, -1)

				if len(split) > 0 {
					serverName = split[len(split)-1]
					break
				}
			}
		}
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "fly-mcp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Debugf("Created temporary directory: %s\n", tempDir)

	appName := flag.GetString(ctx, "name")
	if appName == "" {
		appName = serverName
	}

	appDir := filepath.Join(tempDir, appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	log.Debugf("Created app directory: %s\n", appDir)

	if err := os.Chdir(appDir); err != nil {
		return fmt.Errorf("failed to change to app directory: %w", err)
	}

	args := []string{"launch", "--yes", "--no-deploy"}

	if image != "" {
		dockerfile := []string{"FROM " + image}

		if image != "flyio/mcp" {
			dockerfile = append(dockerfile, "COPY --from=flyio/flyctl /flyctl /usr/bin/flyctl")
			entrypoint = append([]string{"/usr/bin/flyctl", "mcp", "wrap", "--"}, entrypoint...)
		}

		dockerfile = append(dockerfile, setup...)

		jsonData, err := json.Marshal(entrypoint)
		if err != nil {
			return fmt.Errorf("failed to marshal entrypoint to JSON: %w", err)
		}
		dockerfile = append(dockerfile, "ENTRYPOINT "+string(jsonData))

		if len(cmdParts) > 0 {
			jsonData, err := json.Marshal(cmdParts)
			if err != nil {
				return fmt.Errorf("failed to marshal command parts to JSON: %w", err)
			}

			dockerfile = append(dockerfile, "CMD "+string(jsonData))
		}

		dockerfileContent := strings.Join(dockerfile, "\n") + "\n"

		fmt.Println(dockerfileContent)

		if err := os.WriteFile(filepath.Join(appDir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
			return fmt.Errorf("failed to create Dockerfile: %w", err)
		}

		log.Debug("Created Dockerfile")
	} else {
		args = append(args, "--command", command, "--image", "flyio/mcp")
	}

	if flycast := flag.GetBool(ctx, "flycast"); flycast {
		args = append(args, "--flycast")
	}

	if autoStop := flag.GetString(ctx, "auto-stop"); autoStop != "" {
		args = append(args, "--auto-stop", autoStop)
	}

	if region := flag.GetString(ctx, "region"); region != "" {
		args = append(args, "--region", region)
	}

	if org := flag.GetString(ctx, "org"); org != "" {
		args = append(args, "--org", org)
	}

	if vmCpuKind := flag.GetString(ctx, "vm-cpu-kind"); vmCpuKind != "" {
		args = append(args, "--vm-cpu-kind", vmCpuKind)
	}

	if vmCpus := flag.GetInt(ctx, "vm-cpus"); vmCpus != 0 {
		args = append(args, "--vm-cpus", fmt.Sprintf("%d", vmCpus))
	}

	if vmGpuKind := flag.GetString(ctx, "vm-gpu-kind"); vmGpuKind != "" {
		args = append(args, "--vm-gpu-kind", vmGpuKind)
	}

	if vmGpus := flag.GetInt(ctx, "vm-gpus"); vmGpus != 0 {
		args = append(args, "--vm-gpus", fmt.Sprintf("%d", vmGpus))
	}

	if vmMemory := flag.GetString(ctx, "vm-memory"); vmMemory != "" {
		args = append(args, "--vm-memory", vmMemory)
	}

	if vmSize := flag.GetString(ctx, "vm-size"); vmSize != "" {
		args = append(args, "--vm-size", vmSize)
	}

	if hostDedicationId := flag.GetString(ctx, "host-dedication-id"); hostDedicationId != "" {
		args = append(args, "--host-dedication-id", hostDedicationId)
	}

	volumes := flag.GetStringSlice(ctx, "volume")
	if len(volumes) > 0 {
		args = append(args, "--volume", strings.Join(volumes, ","))
	}

	// Run fly launch, but don't deploy
	if err := flyctl(args...); err != nil {
		return fmt.Errorf("failed to run 'fly launch': %w", err)
	}

	log.Debug("Launched fly application")

	args = []string{}

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

	tmpConfig := filepath.Join(tempDir, "mcpConfig.json")
	if flag.GetBool(ctx, "inspector") {
		// If the inspector flag is set, capture the MCP client configuration
		log.Debug("Adding inspector to MCP client configuration")
		args = append(args, "--config", tmpConfig)
	}

	if len(args) == 0 {
		log.Debug("No MCP client configuration flags provided")
	} else {
		args = append([]string{"mcp", "add"}, args...)
		args = append(args, "--name", serverName)

		if user := flag.GetString(ctx, "user"); user != "" {
			args = append(args, "--user", user)
		}

		if password := flag.GetString(ctx, "password"); password != "" {
			args = append(args, "--password", password)
		}

		if bearer := flag.GetBool(ctx, "bearer-token"); bearer {
			args = append(args, "--bearer-token")
		}

		if flycast := flag.GetBool(ctx, "flycast"); flycast {
			args = append(args, "--flycast")
		}

		// Run 'fly mcp add ...'
		if err := flyctl(args...); err != nil {
			return fmt.Errorf("failed to run 'fly mcp add': %w", err)
		}
	}

	// Add secrets to the app
	if secrets := flag.GetStringArray(ctx, "secret"); len(secrets) > 0 {
		parsedSecrets, err := cmdutil.ParseKVStringsToMap(secrets)
		if err != nil {
			return fmt.Errorf("failed parsing secrets: %w", err)
		}

		args = []string{"secrets", "set"}
		for k, v := range parsedSecrets {
			args = append(args, fmt.Sprintf("%s=%s", k, v))
		}

		// Run 'fly secrets set ...'
		if err := flyctl(args...); err != nil {
			return fmt.Errorf("failed to run 'fly secrets set': %w", err)
		}
	}

	args = []string{"deploy", "--ha=false"}

	for _, file := range flag.GetStringArray(ctx, "file-local") {
		if file != "" {
			args = append(args, "--file-local", file)
		}
	}

	for _, file := range flag.GetStringArray(ctx, "file-literal") {
		if file != "" {
			args = append(args, "--file-literal", file)
		}
	}

	for _, file := range flag.GetStringArray(ctx, "file-secret") {
		if file != "" {
			args = append(args, "--file-secret", file)
		}
	}

	// Deploy to a single machine
	if err := flyctl(args...); err != nil {
		return fmt.Errorf("failed to run 'fly launch': %w", err)
	}

	log.Debug("Successfully completed MCP server launch and configuration")

	// If the inspector flag is set, run the MCP inspector
	if flag.GetBool(ctx, "inspector") {
		server, err := configExtract(ConfigPath{Path: tmpConfig, ConfigName: "mcpServers"}, serverName)
		if err != nil {
			return fmt.Errorf("failed to extract config: %w", err)
		}

		args := []string{"@modelcontextprotocol/inspector@latest"}
		args = append(args, server["command"].(string))

		// Convert []interface{} to []string
		rawArgs, _ := server["args"].([]interface{})
		for _, v := range rawArgs {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}

		inspectorCmd := exec.Command("npx", args...)
		inspectorCmd.Env = os.Environ()
		inspectorCmd.Stdout = os.Stdout
		inspectorCmd.Stderr = os.Stderr
		inspectorCmd.Stdin = os.Stdin
		if err := inspectorCmd.Run(); err != nil {
			return fmt.Errorf("failed to run MCP inspector: %w", err)
		}
		log.Debug("MCP inspector launched")
	}

	return nil
}
