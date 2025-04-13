package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func NewProxy() *cobra.Command {
	const (
		short = "[experimental] Start an MCP proxy client"
		long  = short + "\n"
		usage = "proxy"
	)

	cmd := command.New(usage, short, long, runProxy)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.String{
			Name:        "url",
			Description: "URL of the MCP wrapper server",
		},
		flag.String{
			Name:        "user",
			Description: "[optional] User to authenticate with",
			Shorthand:   "u",
		},
		flag.String{
			Name:        "password",
			Description: "[optional] Password to authenticate with",
			Shorthand:   "p",
		},
	)

	return cmd
}

func runProxy(ctx context.Context) error {
	// Validate inputs
	if flag.GetString(ctx, "url") == "" {
		log.Fatal("--url is required")
	}

	// Configure logging to go to stderr only
	log.SetOutput(os.Stderr)

	// Start the HTTP client
	go func() {
		start := time.Now()
		for {
			getFromServer(ctx)

			// Wait a minimum of 10 seconds before the next request
			elapsed := time.Since(start)
			if elapsed < 10*time.Second {
				time.Sleep(10*time.Second - elapsed)
			}
			start = time.Now()
		}
	}()

	// Start processing stdin
	if err := processStdin(ctx); err != nil {
		log.Fatalf("Error processing stdin: %v", err)
	}

	return nil
}

// ProcessStdin reads messages from stdin and forwards them to the server
func processStdin(ctx context.Context) error {
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

		// Forward raw message to server
		err := sendToServer(ctx, line)
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

func getFromServer(ctx context.Context) error {
	url := flag.GetString(ctx, "url")

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json")

	// Set basic authentication if user is provided
	if flag.GetString(ctx, "user") != "" {
		req.SetBasicAuth(flag.GetString(ctx, "user"), flag.GetString(ctx, "password"))
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

	// Stream response body to stdout
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return fmt.Errorf("error streaming response to stdout: %w", err)
	}

	return nil
}

// SendToServer sends a raw message to the server and returns the raw response
func sendToServer(ctx context.Context, message string) error {
	url := flag.GetString(ctx, "url")

	// Create HTTP request with raw message
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Set basic authentication if user is provided
	if flag.GetString(ctx, "user") != "" {
		req.SetBasicAuth(flag.GetString(ctx, "user"), flag.GetString(ctx, "password"))
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
