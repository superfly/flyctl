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
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

// This program is a simple HTTP server that forwards POST requests to an MCP stdio program,
// and streams the program's output back to the client. It uses Server-Sent Events (SSE)
// to push updates from the server to the client.
//
// It is a streamlined version of the MCP proxy server, focusing on a single session:
// See https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http

// Server handles HTTP requests and communicates with the remote program
type Server struct {
	port     int
	mcp      string
	token    string
	user     string
	password string
	private  bool
	cmd      *exec.Cmd
	args     []string
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	mutex    sync.Mutex
	client   chan string
}

func NewWrap() *cobra.Command {
	const (
		short = "[experimental] Wrap an MCP stdio program"
		long  = short + `. Options passed after double dashes ("--") will be passed to the MCP program. If user is specified, HTTP authentication will be required.` + "\n"
		usage = "wrap"
	)

	cmd := command.New(usage, short, long, runWrap)
	cmd.Args = cobra.ExactArgs(0)

	flag.Add(cmd,
		flag.Int{
			Name:        "port",
			Description: "Port to listen on.",
			Default:     8080,
			Shorthand:   "p",
		},
		flag.String{
			Name:        "mcp",
			Description: "Path to the stdio MCP program to be wrapped.",
			Shorthand:   "m",
		},
		flag.String{
			Name:        "bearer-token",
			Description: "Bearer token to authenticate with. Defaults to the value of the FLY_MCP_BEARER_TOKEN environment variable.",
		},
		flag.String{
			Name:        "user",
			Description: "User to authenticate with. Defaults to the value of the FLY_MCP_USER environment variable.",
		},
		flag.String{
			Name:        "password",
			Description: "Password to authenticate with. Defaults to the value of the FLY_MCP_PASSWORD environment variable.",
		},
		flag.Bool{
			Name:        "private",
			Description: "Use private networking.",
		},
	)

	return cmd
}

func runWrap(ctx context.Context) error {
	token, _ := os.LookupEnv("FLY_MCP_BEARER_TOKEN")
	user, _ := os.LookupEnv("FLY_MCP_USER")
	password, _ := os.LookupEnv("FLY_MCP_PASSWORD")
	_, private := os.LookupEnv("FLY_MCP_PRIVATE")

	if token == "" {
		token = flag.GetString(ctx, "bearer-token")
	}

	if user == "" {
		user = flag.GetString(ctx, "user")
	}

	if password == "" {
		password = flag.GetString(ctx, "password")
	}

	// Create server
	server := &Server{
		port:     flag.GetInt(ctx, "port"),
		token:    token,
		user:     user,
		password: password,
		private:  flag.GetBool(ctx, "private") || private,
		mcp:      flag.GetString(ctx, "mcp"),
		args:     flag.ExtraArgsFromContext(ctx),
		client:   nil,
	}

	// if user and password are not set, try to get them from environment variables
	if server.user == "" {
		server.user = os.Getenv("FLY_MCP_USER")
	}

	if server.password == "" {
		server.password = os.Getenv("FLY_MCP_PASSWORD")
	}

	// Start the program
	if err := server.StartProgram(); err != nil {
		log.Fatalf("Error starting program: %v", err)
	}
	defer server.StopProgram()

	// Start reading from the program's stdout
	go server.ReadFromProgram()

	// Set up HTTP server
	http.HandleFunc("/", server.HandleHTTPRequest)
	address := fmt.Sprintf(":%d", server.port)

	log.Printf("Starting server on %s, forwarding to stdio MCP: %s", address, server.mcp)
	if err := http.ListenAndServe(address, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}

	return nil
}

// StartProgram starts the remote program and connects to its stdin/stdout
func (s *Server) StartProgram() error {
	command := s.mcp
	args := s.args

	if command == "" {
		if len(args) == 0 {
			return fmt.Errorf("no command specified")
		}

		command = args[0]
		args = args[1:]
	}

	cmd := exec.Command(command, args...)

	// Get stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error getting stdin pipe: %w", err)
	}
	s.stdin = stdin

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error getting stdout pipe: %w", err)
	}
	s.stdout = stdout

	// Redirect stderr to our stderr
	cmd.Stderr = os.Stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting program: %w", err)
	}

	s.cmd = cmd

	// Monitor program exit
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("Program exited with error: %v", err)
		} else {
			log.Println("Program exited normally")
		}
	}()

	return nil
}

// StopProgram stops the remote program
func (s *Server) StopProgram() {
	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("Stopping program")
		if err := s.cmd.Process.Kill(); err != nil {
			log.Printf("Error killing program: %v", err)
		}
	}
}

// ReadFromProgram continuously reads from the program's stdout
func (s *Server) ReadFromProgram() {
	stp := make(chan os.Signal, 1)
	signal.Notify(stp, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stp
		s.StopProgram()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(s.stdout)
	const (
		defaultBufSize  = bufio.MaxScanTokenSize // 64KiB
		maxResponseSize = 10 * 1024 * 1024       // 10MiB
	)
	scanner.Buffer(make([]byte, 0, defaultBufSize), maxResponseSize)
	for scanner.Scan() {
		line := scanner.Text() + "\n"

		// Forward message to waiting client
		s.mutex.Lock()
		if s.client != nil {
			s.client <- line
		} else {
			log.Printf("No client waiting")
		}
		s.mutex.Unlock()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from program: %v", err)
	} else {
		log.Println("Program output stream closed")
	}

	// Close stdin to signal EOF to the program
	if err := s.stdin.Close(); err != nil {
		log.Printf("Error closing stdin: %v", err)
	}
	// Close stdout to signal EOF to the program
	if err := s.stdout.Close(); err != nil {
		log.Printf("Error closing stdout: %v", err)
	}
}

// HandleHTTPRequest handles incoming HTTP requests
func (s *Server) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	debugLog := os.Getenv("LOG_LEVEL") == "debug"

	// Access logging
	if debugLog {
		log.Printf("Incoming request: %s %s", r.Method, r.URL.Path)
		for name, values := range r.Header {
			if strings.EqualFold(name, "Authorization") {
				log.Printf("Header: %s: [REDACTED]", name)
			} else {
				log.Printf("Header: %s: %v", name, values)
			}
		}
	}

	if s.private {
		clientIP := r.Header.Get("Fly-Client-Ip")
		if clientIP != "" && !strings.HasPrefix(clientIP, "fdaa:") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	if s.token != "" {
		// Check for bearer token
		bearerToken := r.Header.Get("Authorization")
		if bearerToken == "" || !strings.HasPrefix(bearerToken, "Bearer ") || strings.TrimSpace(bearerToken[7:]) != s.token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if s.user != "" {
		// Check for basic authentication
		user, password, ok := r.BasicAuth()
		if !ok || user != s.user || password != s.password {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Handle GET requests
	if r.Method == http.MethodGet {

		// Respond to HTML requests with a simple message
		acceptHeader := r.Header.Get("Accept")
		if strings.Contains(acceptHeader, "html") && !strings.Contains(acceptHeader, "json") {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("MCP Server"))
			return
		}

		// Create channel for response
		responseCh := make(chan string, 1)
		s.mutex.Lock()
		if s.client == nil {
			s.client = responseCh
		}
		s.mutex.Unlock()

		if s.client != responseCh {
			// If we already have a client, return an error
			http.Error(w, "Another client is already connected", http.StatusConflict)
			return
		}

		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		w.(http.Flusher).Flush() // Flush headers to the client

		// Stream responses to the client
		for {
			select {
			case response := <-responseCh:
				w.Write([]byte(response))
				w.(http.Flusher).Flush() // Flush the response to the client
			case <-r.Context().Done():
				// Request was cancelled
				s.mutex.Lock()
				s.client = nil
				s.mutex.Unlock()
				return
			}
		}

	} else if r.Method == http.MethodPost {
		if debugLog {
			// Capture request body for logging
			var bodyBuf bytes.Buffer
			r.Body = io.NopCloser(io.TeeReader(r.Body, &bodyBuf))
			log.Printf("Request body: %s", bodyBuf.String())
		}

		// Stream request body to program's stdin, but inspect the last byte
		var lastByte byte
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			if n > 0 {
				lastByte = buf[n-1]
				if _, werr := s.stdin.Write(buf[:n]); werr != nil {
					log.Printf("Error writing to program: %v", werr)
					http.Error(w, fmt.Sprintf("Error writing to program: %v", werr), http.StatusInternalServerError)
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading request body: %v", err)
				http.Error(w, fmt.Sprintf("Error reading request body: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Ensure the last byte is a newline
		if lastByte != '\n' {
			s.stdin.Write([]byte{'\n'})
		}

		if f, ok := s.stdin.(interface{ Flush() error }); ok {
			if err := f.Flush(); err != nil {
				log.Printf("Error flushing stdin: %v", err)
			}
		}

		// Successfully wrote to program
		w.WriteHeader(http.StatusAccepted)
		r.Body.Close()

	} else {
		// Method not allowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
