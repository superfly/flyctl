package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/env"
)

const (
	// SocketPathEnvKey names the environment variable that overrides the path
	// of the socket the agent listens on. StartDaemon exports it to the agent
	// subprocess it spawns so that both ends agree on the path.
	SocketPathEnvKey = "FLY_AGENT_SOCKET_PATH"

	// SocketDirEnvKey names the environment variable carrying the private
	// directory an isolated invocation created for its socket and lock
	// files. The agent subprocess removes it on shutdown.
	SocketDirEnvKey = "FLY_AGENT_SOCKET_DIR"

	// IsolatedEnvKey names the environment variable that, when truthy, makes
	// flyctl run its own agent as a subprocess on a randomly generated socket
	// path instead of sharing the background daemon with other invocations.
	// The isolated agent shuts down when the flyctl invocation that started
	// it exits.
	IsolatedEnvKey = "FLY_AGENT_ISOLATED"

	// TokenModeEnvKey names the environment variable that, when set,
	// overrides the wire_guard_token_mode config key: truthy values make
	// the agent provision WireGuard peers inline at the token gateway with
	// the session's macaroons instead of registering them through the API.
	TokenModeEnvKey = "FLY_WIREGUARD_TOKEN_MODE"

	// TokenGatewayEnvKey names the environment variable that overrides the
	// token gateway hostname (wg.DefaultTokenGateway), e.g. to point at a
	// staging gateway.
	TokenGatewayEnvKey = "FLY_WIREGUARD_GATEWAY"
)

// TokenModeEnabled reports whether the agent will build tunnels via the
// token gateway: the wire_guard_token_mode config key, overridden by
// FLY_WIREGUARD_TOKEN_MODE.
func TokenModeEnabled() bool {
	if os.Getenv(TokenModeEnvKey) != "" {
		return env.IsTruthy(TokenModeEnvKey)
	}

	return viper.GetBool(flyctl.ConfigWireGuardTokenMode)
}

// Isolated reports whether this invocation runs its own agent subprocess on
// a private socket instead of sharing the background daemon.
func Isolated() bool {
	return env.IsTruthy(IsolatedEnvKey)
}

var socketGenMu sync.Mutex

// PathToSocket returns the path of the socket the agent listens on: the
// value of FLY_AGENT_SOCKET_PATH if set, a randomly generated per-process
// path in isolated mode (exported via FLY_AGENT_SOCKET_PATH so subprocesses
// inherit it), or fly-agent.sock in the config directory.
func PathToSocket() string {
	if path := os.Getenv(SocketPathEnvKey); path != "" {
		return path
	}

	if Isolated() {
		socketGenMu.Lock()
		defer socketGenMu.Unlock()

		if path := os.Getenv(SocketPathEnvKey); path != "" {
			return path
		}

		path := randomSocketPath()
		os.Setenv(SocketPathEnvKey, path)

		return path
	}

	dir, err := helpers.GetConfigDirectory()
	if err != nil {
		panic(err)
	}

	return filepath.Join(dir, "fly-agent.sock")
}

// SocketPathOverride returns the socket path when one is in effect, either
// explicitly via FLY_AGENT_SOCKET_PATH or generated in isolated mode, and the
// empty string when the default path applies.
func SocketPathOverride() string {
	if os.Getenv(SocketPathEnvKey) != "" || Isolated() {
		return PathToSocket()
	}

	return ""
}

// randomSocketPath creates a private (0700) directory for this invocation's
// socket and lock files. The socket must not sit in the shared temp dir
// itself: a unix socket takes its mode from the umask, and on a shared box
// that could let other users drive the agent.
func randomSocketPath() string {
	dir, err := os.MkdirTemp("", "fly-agent-")
	if err != nil {
		panic(fmt.Sprintf("can't create agent socket directory: %s", err))
	}
	os.Setenv(SocketDirEnvKey, dir)

	// keep the path short: unix socket paths are limited to ~100 bytes
	return filepath.Join(dir, "agent.sock")
}

// SocketDirToRemove returns the private socket directory this agent should
// remove on shutdown, or "" when the socket doesn't live in one.
func SocketDirToRemove() string {
	dir := os.Getenv(SocketDirEnvKey)
	if dir == "" {
		return ""
	}

	if rel, err := filepath.Rel(dir, PathToSocket()); err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}

	return dir
}

type Instances struct {
	Labels    []string
	Addresses []string
}
