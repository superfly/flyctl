# Flyctl Issue #2844 Analysis

## Problem
The machine restart policy flag uses `on-fail` but should be `on-failure` to match:
- Docker's restart policy standard
- Fly.io's Machines API

## Current Code Location
File: `internal/command/machine/run.go`
Line: ~791 (in the switch statement for restart policy)

## Current Code:
```go
case "on-fail":
    machineConf.Restart = &fly.MachineRestart{
        Policy: fly.MachineRestartPolicyOnFailure,
    }
```

## Solution
Change `"on-fail"` to `"on-failure"` in the case statement.

## Also Need to Update
- Flag description on line ~106 which mentions `'on-fail'`
- Documentation text that refers to the old value

This is a simple string replacement that maintains backward compatibility concerns mentioned in the issue.
