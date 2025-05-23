package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
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

type ProxyInfo struct {
	url         string
	bearerToken string
	user        string
	password    string
}

func runProxy(ctx context.Context) error {
	proxyInfo := ProxyInfo{
		url:         flag.GetString(ctx, "url"),
		bearerToken: flag.GetString(ctx, "bearer-token"),
		user:        flag.GetString(ctx, "user"),
		password:    flag.GetString(ctx, "password"),
	}

	return runProxyOrInspect(ctx, proxyInfo, flag.GetBool(ctx, "inspector"))
}

func runInspect(ctx context.Context) error {
	proxyInfo := ProxyInfo{
		url:         flag.GetString(ctx, "url"),
		bearerToken: flag.GetString(ctx, "bearer-token"),
		user:        flag.GetString(ctx, "user"),
		password:    flag.GetString(ctx, "password"),
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

		if proxyInfo.url == "" {
			proxyInfo.url, _ = mcpConfig["url"].(string)
		}
		if proxyInfo.bearerToken == "" {
			proxyInfo.bearerToken, _ = mcpConfig["bearer-token"].(string)
		}
		if proxyInfo.user == "" {
			proxyInfo.user, _ = mcpConfig["user"].(string)
		}
		if proxyInfo.password == "" {
			proxyInfo.password, _ = mcpConfig["password"].(string)
		}
	} else if len(configPaths) > 1 {
		return fmt.Errorf("multiple MCP client configuration files specifed. Please specify at most one")
	}

	return runProxyOrInspect(ctx, proxyInfo, true)
}

func runProxyOrInspect(ctx context.Context, proxyInfo ProxyInfo, inspect bool) error {

	// If no URL is provided, try to get it from the app config
	// If that fails, return an error
	if proxyInfo.url == "" {
		appConfig := appconfig.ConfigFromContext(ctx)

		if appConfig != nil {
			appUrl := appConfig.URL()
			if appUrl != nil {
				proxyInfo.url = appUrl.String()
			}
		}

		if proxyInfo.url == "" {
			log.Fatal("The app config could not be found and no URL was provided")
		}
	}

	if inspect {
		flyctl, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to find executable: %w", err)
		}

		args := []string{"@modelcontextprotocol/inspector@latest", flyctl, "mcp", "proxy", "--url", proxyInfo.url}

		if proxyInfo.bearerToken != "" {
			args = append(args, "--bearer-token", proxyInfo.bearerToken)
		}
		if proxyInfo.user != "" {
			args = append(args, "--user", proxyInfo.user)
		}
		if proxyInfo.password != "" {
			args = append(args, "--password", proxyInfo.password)
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

	url, proxyCmd, err := resolveProxy(ctx, proxyInfo.url)
	if err != nil {
		log.Fatalf("Error resolving proxy URL: %v", err)
	}

	proxyInfo.url = url

	// Configure logging to go to stderr only
	log.SetOutput(os.Stderr)

	err = waitForServer(ctx, proxyInfo)
	if err != nil {
		log.Fatalf("Error waiting for server: %v", err)
	}

	// Store whether the SSE connection is ready
	// This may become unready if the connection is closed
	ready := false
	readyMutex := sync.Mutex{}
	readyCond := sync.NewCond(&readyMutex)

	// Start the HTTP client
	go func() {
		start := time.Now()
		for {
			getFromServer(ctx, proxyInfo, &ready, readyCond)

			// Ready should be set to false when the connection is closed
			readyCond.L.Lock()
			ready = false
			readyCond.Broadcast()
			readyCond.L.Unlock()

			// Wait a minimum of 10 seconds before the next request
			elapsed := time.Since(start)
			if elapsed < 10*time.Second {
				time.Sleep(10*time.Second - elapsed)
			}
			start = time.Now()
		}
	}()

	// Start processing stdin
	if err := processStdin(ctx, proxyInfo, &ready, readyCond); err != nil {
		log.Fatalf("Error processing stdin: %v", err)
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

// waitForServer waits for the server to be up and running
func waitForServer(ctx context.Context, proxyInfo ProxyInfo) error {
	// Continue to post nothing until the server is up
	delay := 100 * time.Millisecond
	var err error
	for delay < 60*time.Second {
		err = sendToServer(ctx, "", proxyInfo)

		if err == nil {
			break
		} else if !strings.Contains(err.Error(), "connection refused") {
			log.Printf("Error sending message to server: %v", err)
			break
		}

		time.Sleep(delay)
		delay *= 2
	}

	return err
}

// ProcessStdin reads messages from stdin and forwards them to the server
func processStdin(ctx context.Context, proxyInfo ProxyInfo, ready *bool, readyCond *sync.Cond) error {
	stp := make(chan os.Signal, 1)
	signal.Notify(stp, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stp
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text() + "\n"

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Wait for the server to be ready
		readyCond.L.Lock()
		for !*ready {
			readyCond.Wait()
		}
		readyCond.L.Unlock()

		// Forward raw message to server
		err := sendToServer(ctx, line, proxyInfo)
		if err != nil {
			// Log error but continue processing
			log.Printf("Error sending message to server: %v", err)
			// We could format an error message here, but since we're operating at the raw string level,
			// we'll return a generic error JSON
			errMsg := fmt.Sprintf(`{"type":"error","content":"Failed to send to server: %v"}`, err)
			fmt.Fprintln(os.Stdout, errMsg)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from stdin: %w", err)
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

	cmd := exec.Command(os.Args[0], "proxy", ports, remoteHost, "--quiet", "--app", appName)
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

// getFromServer sends a GET request to the server and streams the response to stdout
func getFromServer(ctx context.Context, proxyInfo ProxyInfo, ready *bool, readyCond *sync.Cond) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", proxyInfo.url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json")

	// Set basic authentication if bearer token or user is provided
	if proxyInfo.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+proxyInfo.bearerToken)
	} else if proxyInfo.user != "" {
		req.SetBasicAuth(proxyInfo.user, proxyInfo.password)
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s (status %d)", resp.Status, resp.StatusCode)
	}

	// We're now ready to receive messages
	readyCond.L.Lock()
	*ready = true
	readyCond.Broadcast()
	readyCond.L.Unlock()

	// Stream response body to stdout
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return fmt.Errorf("error streaming response to stdout: %w", err)
	}

	return nil
}

// SendToServer sends a raw message to the server and returns the raw response
func sendToServer(ctx context.Context, message string, proxyInfo ProxyInfo) error {
	// Create HTTP request with raw message
	req, err := http.NewRequest("POST", proxyInfo.url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Set basic authentication if bearer token or user is provided
	if proxyInfo.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+proxyInfo.bearerToken)
	} else if proxyInfo.user != "" {
		req.SetBasicAuth(proxyInfo.user, proxyInfo.password)
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusAccepted {
		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response: %w", err)
		}

		return fmt.Errorf("server returned error: %s (status %d)", body, resp.StatusCode)
	}

	// Request was accepted
	return nil
}
