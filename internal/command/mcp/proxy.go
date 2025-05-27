package mcp

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	mcpProxy "github.com/superfly/flyctl/internal/command/mcp/proxy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

var sharedProxyFlags = flag.Set{
	flag.App(),

	flag.String{
		Name:        "url",
		Description: "URL of the MCP wrapper server",
	},
	flag.String{
		Name:        "bearer-token",
		Description: "Bearer token to authenticate with",
	},
	flag.String{
		Name:        "user",
		Description: "User to authenticate with",
		Shorthand:   "u",
	},
	flag.String{
		Name:        "password",
		Description: "Password to authenticate with",
		Shorthand:   "p",
	},
	flag.String{
		Name:        flagnames.BindAddr,
		Shorthand:   "b",
		Default:     "127.0.0.1",
		Description: "Local address to bind to",
	},
	flag.String{
		Name:        "instance",
		Description: "Use fly-force-instance-id to connect to a specific instance",
	},
}

func NewProxy() *cobra.Command {
	const (
		short = "[experimental] Start an MCP proxy client"
		long  = short + "\n"
		usage = "proxy"
	)

	cmd := command.New(usage, short, long, runProxy, command.LoadAppNameIfPresent)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		sharedProxyFlags,
		flag.Bool{
			Name:        "inspector",
			Description: "Launch MCP inspector: a developer tool for testing and debugging MCP servers",
			Default:     false,
			Shorthand:   "i",
		},
	)

	return cmd
}

func NewInspect() *cobra.Command {
	const (
		short = "[experimental] Inspect a MCP stdio server"
		long  = short + "\n"
		usage = "inspect"
	)

	cmd := command.New(usage, short, long, runInspect, command.LoadAppNameIfPresent)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		sharedProxyFlags,
		flag.String{
			Name:        "server",
			Description: "Name of the MCP server in the MCP client configuration",
		},
		flag.String{
			Name:        "config",
			Description: "Path to the MCP client configuration file",
		},
	)

	for client, name := range McpClients {
		flag.Add(cmd,
			flag.Bool{
				Name:        client,
				Description: "Use the configuration for " + name + " client",
			},
		)
	}

	return cmd
}

func runProxy(ctx context.Context) error {
	proxyInfo := mcpProxy.ProxyInfo{
		Url:         flag.GetString(ctx, "url"),
		BearerToken: flag.GetString(ctx, "bearer-token"),
		User:        flag.GetString(ctx, "user"),
		Password:    flag.GetString(ctx, "password"),
		Instance:    flag.GetString(ctx, "instance"),
	}

	return runProxyOrInspect(ctx, proxyInfo, flag.GetBool(ctx, "inspector"))
}

func runInspect(ctx context.Context) error {
	proxyInfo := mcpProxy.ProxyInfo{
		Url:         flag.GetString(ctx, "url"),
		BearerToken: flag.GetString(ctx, "bearer-token"),
		User:        flag.GetString(ctx, "user"),
		Password:    flag.GetString(ctx, "password"),
		Instance:    flag.GetString(ctx, "instance"),
	}

	server := flag.GetString(ctx, "server")

	configPaths, err := ListConfigPaths(ctx, true)
	if err != nil {
		return err
	}

	if len(configPaths) == 1 {
		mcpConfig, err := configExtract(configPaths[0], server)
		if err != nil {
			return err
		}

		if proxyInfo.Url == "" {
			proxyInfo.Url, _ = mcpConfig["url"].(string)
		}
		if proxyInfo.BearerToken == "" {
			proxyInfo.BearerToken, _ = mcpConfig["bearer-token"].(string)
		}
		if proxyInfo.User == "" {
			proxyInfo.User, _ = mcpConfig["user"].(string)
		}
		if proxyInfo.Password == "" {
			proxyInfo.Password, _ = mcpConfig["password"].(string)
		}
	} else if len(configPaths) > 1 {
		return fmt.Errorf("multiple MCP client configuration files specifed. Please specify at most one")
	}

	return runProxyOrInspect(ctx, proxyInfo, true)
}

func runProxyOrInspect(ctx context.Context, proxyInfo mcpProxy.ProxyInfo, inspect bool) error {

	// If no URL is provided, try to get it from the app config
	// If that fails, return an error
	if proxyInfo.Url == "" {
		appConfig := appconfig.ConfigFromContext(ctx)

		if appConfig != nil {
			appUrl := appConfig.URL()
			if appUrl != nil {
				proxyInfo.Url = appUrl.String()
			}
		}

		if proxyInfo.Url == "" {
			log.Fatal("The app config could not be found and no URL was provided")
		}
	}

	if inspect {
		flyctl, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to find executable: %w", err)
		}

		args := []string{"@modelcontextprotocol/inspector@latest", flyctl, "mcp", "proxy", "--url", proxyInfo.Url}

		if proxyInfo.BearerToken != "" {
			args = append(args, "--bearer-token", proxyInfo.BearerToken)
		}
		if proxyInfo.User != "" {
			args = append(args, "--user", proxyInfo.User)
		}
		if proxyInfo.Password != "" {
			args = append(args, "--password", proxyInfo.Password)
		}

		// Launch MCP inspector
		cmd := exec.Command("npx", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to launch MCP inspector: %w", err)
		}
		return nil
	}

	url, proxyCmd, err := resolveProxy(ctx, proxyInfo.Url)
	if err != nil {
		log.Fatalf("Error resolving proxy URL: %v", err)
	}

	proxyInfo.Url = url

	// Configure logging to go to stderr only
	log.SetOutput(os.Stderr)

	err = mcpProxy.Passthru(ctx, proxyInfo)
	if err != nil {
		log.Fatal(err)
	}

	// Kill the proxy process if it was started
	if proxyCmd != nil {
		if err := proxyCmd.Process.Kill(); err != nil {
			log.Printf("Error killing proxy process: %v", err)
		}
		proxyCmd.Wait()
	}

	return nil
}

// resolveProxy starts the proxy process and returns the new URL
func resolveProxy(ctx context.Context, originalUrl string) (string, *exec.Cmd, error) {
	appName := flag.GetString(ctx, "app")

	parsedURL, err := url.Parse(originalUrl)
	if err != nil {
		return "", nil, fmt.Errorf("error parsing URL: %w", err)
	}

	// If the app name is not provided, try to extract it from the URL
	if appName == "" {
		hostname := parsedURL.Hostname()
		if strings.HasSuffix(hostname, ".internal") || strings.HasSuffix(hostname, ".flycast") {
			// Split the hostname by dots
			parts := strings.Split(hostname, ".")

			// The app name should be the part before the last segment (internal or flycast)
			if len(parts) >= 2 {
				appName = parts[len(parts)-2]
			} else {
				return originalUrl, nil, nil
			}
		} else {
			return originalUrl, nil, nil
		}
	}

	if parsedURL.Scheme != "http" {
		return "", nil, fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	// get an available port on the local machine
	localPort, err := getAvailablePort()
	if err != nil {
		return "", nil, fmt.Errorf("error getting available port: %w", err)
	}

	remoteHost := parsedURL.Hostname()

	remotePort := parsedURL.Port()
	if remotePort == "" {
		if parsedURL.Scheme == "http" {
			remotePort = "80"
		} else if parsedURL.Scheme == "https" {
			remotePort = "443"
		}
	}

	ports := fmt.Sprintf("%d:%s", localPort, remotePort)

	flyctl, err := os.Executable()
	if err != nil {
		return "", nil, fmt.Errorf("failed to find executable: %w", err)
	}

	cmd := exec.Command(flyctl, "proxy", ports, remoteHost, "--quiet", "--app", appName)
	cmd.Stdin = nil
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Error running subprocess: %v", err)
	}

	bindAddr := flag.GetBindAddr(ctx)

	parsedURL.Host = fmt.Sprintf("%s:%d", bindAddr, localPort)

	return parsedURL.String(), cmd, nil
}

// getAvailablePort finds an available port on the local machine
func getAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")

	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", addr)

	if err != nil {
		return 0, err
	}

	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}
