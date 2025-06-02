package mcpProxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func Passthru(proxyInfo ProxyInfo) error {
	err := waitForServer(proxyInfo)
	if err != nil {
		return fmt.Errorf("error waiting for server: %w", err)
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
			getFromServer(proxyInfo, &ready, readyCond)

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
	if err := processStdin(proxyInfo, &ready, readyCond); err != nil {
		return fmt.Errorf("error processing stdin: %w", err)
	}

	return nil
}

// waitForServer waits for the server to be up and running
func waitForServer(proxyInfo ProxyInfo) error {
	// Continue to post nothing until the server is up
	delay := 100 * time.Millisecond
	var err error
	for delay < 60*time.Second {
		err = sendToServer("", proxyInfo)

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
func processStdin(proxyInfo ProxyInfo, ready *bool, readyCond *sync.Cond) error {
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
		err := sendToServer(line, proxyInfo)
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

// getFromServer sends a GET request to the server and streams the response to stdout
func getFromServer(proxyInfo ProxyInfo, ready *bool, readyCond *sync.Cond) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", proxyInfo.Url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json")

	// Set basic authentication if bearer token or user is provided
	if proxyInfo.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+proxyInfo.BearerToken)
	} else if proxyInfo.User != "" {
		req.SetBasicAuth(proxyInfo.User, proxyInfo.Password)
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
func sendToServer(message string, proxyInfo ProxyInfo) error {
	// Create HTTP request with raw message
	req, err := http.NewRequest("POST", proxyInfo.Url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mcp-bridge-client")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Set basic authentication if bearer token or user is provided
	if proxyInfo.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+proxyInfo.BearerToken)
	} else if proxyInfo.User != "" {
		req.SetBasicAuth(proxyInfo.User, proxyInfo.Password)
	}

	// If requesting a specific instance, set the header
	if proxyInfo.Instance != "" {
		req.Header.Set("Fly-Force-Instance-Id", proxyInfo.Instance)
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
