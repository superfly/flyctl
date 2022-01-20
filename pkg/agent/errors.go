package agent

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
)

type TunnelError struct {
	error
}

func (err *TunnelError) Unwrap() error {
	return err.error
}

func IsTunnelError(err error) bool {
	var e *TunnelError
	return errors.As(err, &e)
}

type HostNotFoundError struct {
	error
}

func (err *HostNotFoundError) Unwrap() error {
	return err.error
}

func IsHostNotFoundError(err error) bool {
	var e *HostNotFoundError
	return errors.As(err, &e)
}

var tunnelContains = []string{
	"i/o timeout",
	"tunnel unavailable",
	"DNS name does not exist",
}

func mapError(err error, slug, host string) error {
	msg := err.Error()

	for _, part := range tunnelContains {
		if strings.Contains(msg, part) {
			return &TunnelError{err}
		}
	}

	if strings.Contains(msg, "no such host") {
		return &HostNotFoundError{err}
	}

	return err
}

func IsAgentStartError(err error) bool {
	var e *AgentStartError
	return errors.As(err, &e)
}

type AgentStartError struct {
	Output string
}

func (e *AgentStartError) Error() string {
	return "failed to start the agent daemon"
}

func (e *AgentStartError) Description() string {
	if e.Output == "" {
		return ""
	}
	var msg strings.Builder
	msg.WriteString("Agent failed to start with the following output:\n")
	r := regexp.MustCompile(`(?m)^`)
	output := cmdutil.StripANSI(e.Output)
	output = r.ReplaceAllString(output, "\t")
	msg.WriteString(output)
	return msg.String()
}

func (e *AgentStartError) Suggestion() string {
	command := aurora.Bold(fmt.Sprintf("%s agent daemon-start", buildinfo.Name()))
	return fmt.Sprintf("Try running the agent with '%s' to see more output. Once the issue preventing startup is fixed you can stop the agent and flyctl will create it as needed", command)
}
