# Bug: Blue-Green Deployment Bypasses Health Checks When Machines Are Stopped

**Investigated:** flyctl `fly deploy --strategy bluegreen`  
**Test app:** `hello-fly-private` (with `CRASH_NOW=1` env var causing immediate crash)  
**Symptom:** Deployment reports success and shows "1/1 passing" health checks even though the app crashes on startup and all machines end up stopped.

---

## The Short Answer

When the existing **blue machines** are in a **`stopped`** state (e.g. because `auto_stop_machines = 'stop'` stopped them due to no traffic), their `launchInput.SkipLaunch = true` flag gets **silently inherited by the green machines**. Green machines are then created but never started, their health is never verified, and they are marked as `{Total: 1, Passing: 1}` — instantly and unconditionally — before the old machines are destroyed.

---

## The Full Causal Chain

### Step 1 — `skipLaunch()` returns `true` for stopped machines

```go
// internal/command/deploy/machines_launchinput.go
func skipLaunch(origMachineRaw *fly.Machine, mConfig *fly.MachineConfig) bool {
    state := "<not-set>"
    if origMachineRaw != nil {
        state = origMachineRaw.State
    }

    switch {
    case slices.Contains([]string{fly.MachineStateStarted, "starting", "failed"}, state):
        return false          // ← only these three states skip the skip
    case len(mConfig.Standbys) > 0:
        return true
    case origMachineRaw == nil:
        return false
    }

    return true               // ← everything else (stopped, suspended, created, …) → SkipLaunch = true
}
```

With `auto_stop_machines = 'stop'` and `min_machines_running = 0`, Fly's platform automatically stops machines after they become idle. When a new deploy is triggered, the existing (blue) machines may already be in `stopped` state. `skipLaunch` sees `state = "stopped"` and falls through to `return true`.

**This is correct behavior for rolling/immediate deployments** — you don't want to force-start a stopped machine just to update its config.

### Step 2 — `CreateGreenMachines` inherits `SkipLaunch` without resetting it

```go
// internal/command/deploy/strategy_bluegreen.go
func (bg *blueGreen) CreateGreenMachines(ctx context.Context) error {
    for _, mach := range bg.blueMachines {
        p.Go(func() error {
            launchInput := mach.launchInput          // ← copy of blue machine's input
            launchInput.SkipServiceRegistration = true
            // SkipLaunch is NOT reset here — it inherits from the blue machine!

            newMachineRaw, err := bg.flaps.Launch(ctx, bg.app.Name, *launchInput)
            // ...
            bg.greenMachines = append(bg.greenMachines, &machineUpdateEntry{greenMachine, launchInput})
        })
    }
}
```

The green machine's `launchInput` is a shallow copy of the blue machine's `launchInput`. Because `SkipLaunch` is never explicitly set to `false`, the green machine is created with `SkipLaunch = true` — meaning **the Fly API creates the machine but never starts it** (it stays in `created`/`stopped` state).

### Step 3 — `WaitForGreenMachinesToBeStarted` trivially succeeds

```go
// strategy_bluegreen.go
for _, gm := range bg.greenMachines {
    if gm.launchInput.SkipLaunch {
        machineIDToState[id] = "started"   // ← instantly marked as started, no waiting
        continue
    }
    go func(lm machine.LeasableMachine) {
        err := machine.WaitForStartOrStop(...)  // ← never reached
    }(gm.leasableMachine)
}
```

Every green machine has `SkipLaunch = true`, so they all get immediately recorded as `"started"` in the state map. The output reads:

```
Waiting for all green machines to start
  Machine d896005c5015e8 [app] - started     ← machine was never actually started
  Machine 6830971f5706e8 [app] - started
```

### Step 4 — `WaitForGreenMachinesToBeHealthy` trivially succeeds

```go
// strategy_bluegreen.go
for _, gm := range bg.greenMachines {
    if gm.launchInput.SkipLaunch {
        // Unconditionally pre-mark as fully healthy
        machineIDToHealthStatus[gm.leasableMachine.FormattedMachineId()] =
            &fly.HealthCheckStatus{Total: 1, Passing: 1}
        continue
    }
    // ... real health checking goroutine — never reached
}
```

All green machines are immediately slotted into `machineIDToHealthStatus` as `{Total: 1, Passing: 1}`. No goroutine is ever started. No API poll for actual health occurs. The first call to `allMachinesHealthy` sees every machine as passing and returns `true`.

The output reads:

```
Waiting for all green machines to be healthy
  Machine d896005c5015e8 [app] - 1/1 passing  ← fabricated, machine is stopped
  Machine 6830971f5706e8 [app] - 1/1 passing
```

### Step 5 — Green machines are "uncordoned" (made ready for traffic) while stopped

`MarkGreenMachinesAsReadyForTraffic` calls `flaps.Uncordon()` on all green machines
with no `SkipLaunch` guard. The machines get uncordoned (marked as ready for traffic)
even though they are still in `stopped`/`created` state.

The machine's event log confirms the whole sequence:

```
 stopped │ update   │ flyd  │ ...  ← auto-stopped shortly after
 created │ uncordon │ user  │ ...  ← uncordoned while still in created state
 created │ launch   │ user  │ ...  ← created but NOT started (SkipLaunch=true in API call)
 pending │ launch   │ flyd  │ ...
```

### Step 6 — Blue machines are destroyed

Old machines are cordoned, stopped, and destroyed. The new "green" machines are now
the only ones — all stopped, all broken, none serving traffic.

```
fly machines list
ID             │ STATE   │ CHECKS
d896005c5015e8 │ stopped │ 0/2    ← both checks failing
6830971f5706e8 │ stopped │ 0/2
```

---

## Why `auto_stop_machines` Is the Trigger (But Not the Only Way)

With `auto_stop_machines = 'stop'` and `min_machines_running = 0`, any period of zero
incoming traffic will auto-stop all machines. A deploy that immediately follows such a
quiet period will find blue machines in `stopped` state and trigger this bug.

The bug also fires whenever blue machines happen to be stopped for any other reason:
manual `fly machine stop`, a crash that wasn't restarted, a suspended machine, etc.

---

## The Validation Check Does Not Save You

`Deploy()` validates that blue machines have health checks configured before starting
the blue-green flow:

```go
totalMachinesWithChecks := 0
for _, entry := range bg.blueMachines {
    machineChecks := len(entry.launchInput.Config.Checks)
    for _, service := range entry.launchInput.Config.Services {
        machineChecks += len(service.Checks)
    }
    if machineChecks == 0 { continue }
    totalMachinesWithChecks++
}
if totalMachinesWithChecks == 0 && len(bg.blueMachines) != 0 {
    return ErrValidationError   // ← fails correctly if no checks configured
}
```

This check validates that health checks are **configured** (in `Config.Checks` /
`Config.Services[*].Checks`) — which passes fine if you have checks in `fly.toml`. But
it does not validate that green machines will actually be **started and checked**. The
`SkipLaunch` bypass completely avoids the real health checking loop regardless.

---

## The Fix

In `CreateGreenMachines`, explicitly reset `SkipLaunch = false` before launching:

```go
// strategy_bluegreen.go — CreateGreenMachines
launchInput := mach.launchInput
launchInput.SkipServiceRegistration = true
launchInput.SkipLaunch = false          // ← green machines must ALWAYS be started
launchInput.Config.Metadata[fly.MachineConfigMetadataKeyFlyctlBGTag] = bg.timestamp
```

Green machines in a blue-green deployment are always brand-new machines that must be
started, passed health checks, and proven healthy before old machines are destroyed.
The `SkipLaunch` semantics from the blue machine's update context ("don't forcibly
restart a stopped machine") is irrelevant and harmful in this context.

---

## Secondary Issue Also Worth Noting

Even if `SkipLaunch` were always `false`, a second (independent) issue exists in
`WaitForGreenMachinesToBeHealthy`:

```go
if len(gm.leasableMachine.Machine().Checks) == 0 {
    continue   // skip health checking for this machine
}
```

`Machine().Checks` is the **runtime health-check status** field (`[]*MachineCheckStatus`),
not the configuration. For a freshly launched machine it is often empty because health
checks haven't run yet. This check was designed to skip machines with no checks
**configured**, but it is reading the wrong field — it should be checking
`Machine().Config.Checks` (machine-level check config) and
`Machine().Config.Services[*].Checks` (service-level check config), exactly like
`GetMinIntervalAndMinGracePeriod()` does.

In the `SkipLaunch` scenario, this secondary issue is never reached. But if `SkipLaunch`
is fixed, a machine that launches and briefly has an empty `Machine.Checks` runtime
status could still slip through this second guard.

---

## Files Involved

| File | Role |
|------|------|
| `internal/command/deploy/strategy_bluegreen.go` | `CreateGreenMachines` (bug origin), `WaitForGreenMachinesToBeStarted`, `WaitForGreenMachinesToBeHealthy`, `allMachinesHealthy` |
| `internal/command/deploy/machines_launchinput.go` | `skipLaunch()` — sets `SkipLaunch` for blue machines based on their current state |
| `internal/machine/leasable_machine.go` | `WaitForHealthchecksToPass` — same secondary issue (`Machine().Checks` vs `Config.Checks`) |
| `github.com/superfly/fly-go` `machine_types.go` | `Machine.Checks` (runtime status), `Machine.Config.Checks` (config), `AllHealthChecks()`, `RemoveCompatChecks()` |
