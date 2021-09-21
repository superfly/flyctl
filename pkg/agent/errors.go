package agent

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"

	"github.com/logrusorgru/aurora"
)

func IsTunnelError(err error) bool {
	var tunnelError *TunnelError
	return errors.As(err, &tunnelError)
}

type TunnelError struct {
	OrgSlug string
	Err     error
}

func (e *TunnelError) Error() string {
	return fmt.Sprintf("tunnel %s error: %s", e.OrgSlug, e.Err)
}

func (e *TunnelError) Unwrap() error {
	return e.Err
}

func IsHostNotFoundError(err error) bool {
	var notfoundError *HostNotFoundError
	return errors.As(err, &notfoundError)
}

type HostNotFoundError struct {
	OrgSlug string
	Host    string
	Err     error
}

func (e *HostNotFoundError) Error() string {
	return fmt.Sprintf("host %s not found on tunnel %s", e.Host, e.OrgSlug)
}

func (e *HostNotFoundError) Unwrap() error {
	return e.Err
}

func mapResolveError(err error, orgSlug string, host string) error {
	msg := err.Error()
	if strings.Contains(msg, "i/o timeout") {
		return &TunnelError{Err: err, OrgSlug: orgSlug}
	}
	if strings.Contains(msg, "tunnel unavailable") {
		return &TunnelError{Err: err, OrgSlug: orgSlug}
	}
	if strings.Contains(msg, "DNS name does not exist") {
		return &TunnelError{Err: err, OrgSlug: orgSlug}
	}
	if strings.Contains(msg, "no such host") {
		return &HostNotFoundError{Err: err, OrgSlug: orgSlug, Host: host}
	}
	return err
}

func IsAgentStartError(err error) bool {
	var agentErr *AgentStartError
	return errors.As(err, &agentErr)
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
