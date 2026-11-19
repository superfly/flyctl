package tracing

import (
	"context"
	"errors"
)

// WorkflowErrorCode identifies a failure without exporting error text or caller
// input. Values are converted to a fixed vocabulary before entering telemetry.
type WorkflowErrorCode int

const (
	WorkflowErrorUnknown WorkflowErrorCode = iota
	WorkflowErrorCanceled
	WorkflowErrorDeadlineExceeded
	WorkflowErrorInterrupted
	WorkflowErrorPromptNonInteractive
	WorkflowErrorSSHMachineList
	WorkflowErrorSSHMachineLookup
	WorkflowErrorSSHMachineNotFound
	WorkflowErrorSSHMachineNotStarted
	WorkflowErrorSSHMachineAppMismatch
	WorkflowErrorSSHNoStartedMachines
	WorkflowErrorSSHNoMachinesInRegion
	WorkflowErrorSSHMachineRegionMismatch
	WorkflowErrorSSHNoMachinesInProcessGroup
	WorkflowErrorSSHMachineProcessGroupMismatch
	WorkflowErrorSSHSelectionFlags
	WorkflowErrorSSHMachinePrompt
	WorkflowErrorSSHContainerFlags
	WorkflowErrorSSHContainerNotFound
	WorkflowErrorSSHContainerPrompt
	WorkflowErrorSSHDNS
	WorkflowErrorSSHMachineStart
	WorkflowErrorSSHMachineStartWait
)

func (code WorkflowErrorCode) String() string {
	names := [...]string{
		"unknown", "canceled", "deadline_exceeded", "interrupted", "prompt_noninteractive",
		"ssh_machine_list_failed", "ssh_machine_lookup_failed", "ssh_machine_not_found",
		"ssh_machine_not_started", "ssh_machine_app_mismatch", "ssh_no_started_machines",
		"ssh_no_machines_in_region", "ssh_machine_region_mismatch",
		"ssh_no_machines_in_process_group", "ssh_machine_process_group_mismatch",
		"ssh_selection_flags_conflict", "ssh_machine_prompt_failed",
		"ssh_container_flags_conflict", "ssh_container_not_found", "ssh_container_prompt_failed",
		"ssh_dns_lookup_failed", "ssh_machine_start_failed", "ssh_machine_start_wait_failed",
	}
	if code < 0 || int(code) >= len(names) {
		return names[WorkflowErrorUnknown]
	}

	return names[code]
}

type workflowError struct {
	err  error
	code WorkflowErrorCode
}

func (err *workflowError) Error() string { return err.err.Error() }

func (err *workflowError) Unwrap() error { return err.err }

// WithWorkflowErrorCode preserves the error's text and chain for the caller,
// while allowing workflow spans to record a code instead of the error text.
func WithWorkflowErrorCode(err error, code WorkflowErrorCode) error {
	if err == nil {
		return nil
	}
	return &workflowError{err: err, code: code}
}

// WorkflowErrorCodeFor recovers a source-assigned code through error wrappers.
// Mixed joined failures remain unknown rather than borrowing one cause's code.
// Context cancellation is recorded without assuming the user caused it.
func WorkflowErrorCodeFor(err error) WorkflowErrorCode {
	switch err := err.(type) {
	case *workflowError:
		return err.code
	case interface{ Unwrap() []error }:
		children := err.Unwrap()
		if len(children) == 0 {
			return WorkflowErrorUnknown
		}
		code := WorkflowErrorCodeFor(children[0])
		for _, child := range children[1:] {
			if WorkflowErrorCodeFor(child) != code {
				return WorkflowErrorUnknown
			}
		}

		return code
	case interface{ Unwrap() error }:
		return WorkflowErrorCodeFor(err.Unwrap())
	default:
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return WorkflowErrorDeadlineExceeded
		case errors.Is(err, context.Canceled):
			return WorkflowErrorCanceled
		default:
			return WorkflowErrorUnknown
		}
	}
}
