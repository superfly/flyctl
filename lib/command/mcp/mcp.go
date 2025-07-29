package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/command"
)

func New() *cobra.Command {
	const (
		short = `flyctl Model Context Protocol.`

		long = short + "\n"
	)

	cmd := command.New("mcp", short, long, nil)
	// cmd.Hidden = true

	cmd.AddCommand(
		NewProxy(),
		NewInspect(),
		newServer(),
		NewWrap(),

		NewAdd(),
		NewRemove(),

		NewLaunch(),
		NewDestroy(),

		newVolume(),
		newList(),
		newLogs(),
	)

	return cmd
}

func flyctl(args ...string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	log.Debugf("Running:", executable, strings.Join(args, " "))

	cmd := exec.Command(executable, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
