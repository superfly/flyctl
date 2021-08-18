package agent

import (
	"errors"
	"fmt"
	"strings"
)

func IsTunnelError(err error) bool {
	return errors.Is(err, &TunnelError{})
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

func (e *TunnelError) Cause() error {
	return e.Err
}

func IsHostNotFoundError(err error) bool {
	return errors.Is(err, &HostNotFoundError{})
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

func (e *HostNotFoundError) Cause() error {
	return e.Err
}

func mapResolveError(err error, orgSlug string, host string) error {
	msg := err.Error()
	if strings.Contains(msg, "i/o timeout") {
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
