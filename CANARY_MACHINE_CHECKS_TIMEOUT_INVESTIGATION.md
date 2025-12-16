# Canary + Machine Checks Timeout Investigation

## Problem Statement

Tests using canary deployment strategy with machine checks timeout after 15+ minutes in CI:
- `TestFlyDeploy_DeployMachinesCheckCanary` - TIMES OUT
- `TestFlyDeploy_CreateBuilderWDeployToken` - TIMES OUT (suspected)

Similar tests WITHOUT canary strategy pass in ~60 seconds:
- `TestFlyDeploy_DeployMachinesCheck` - PASSES in ~60s

## Test Scenario

Both failing tests follow this pattern:
1. `fly launch --strategy canary` - Creates app + deploys (1 machine, no machine checks yet)
2. Add `[[http_service.machine_checks]]` to fly.toml
3. `fly deploy --buildkit --remote-only` - **HANGS HERE**

## Code Flow Analysis

### Normal Deploy with Machine Checks (Rolling Strategy)
```
deploy.go
  ↓
Build image with BuildKit
  ↓
Update machines (rolling)
  ↓
For each machine update:
  - Run machine checks (test machines)
  - Wait for checks to pass
  ↓
Done (~ 60 seconds)
```

### Canary Deploy with Machine Checks
```
deploy.go
  ↓
Build image with BuildKit
  ↓
deployCanaryMachines() [machines_deploymachinesapp.go:262]
  ├─ Create temporary canary machine (nginx)
  ├─ runTestMachines() [machinebasedtest.go:44]
  │   ├─ createTestMachine() - Create curl container
  │   ├─ Wait for test machine to START (5min timeout)
  │   ├─ Wait for test machine to be DESTROYED (5min timeout)
  │   └─ Check exit code
  └─ Destroy temporary canary machine
  ↓
Actual canary rollout
  ├─ Update first production machine
  └─ Update rest of machines
  ↓
Done (SHOULD be ~2-3 minutes, but HANGS for 15+ minutes)
```

## Hypothesis

The hang occurs during `deployCanaryMachines()`, specifically in `runTestMachines()`.

Possible causes:

### 1. Test Machine Never Starts
- Test machine (curl container) fails to create or start
- But there's a 5-minute timeout for START state
- Should return error, not hang indefinitely

### 2. Test Machine Never Auto-Destroys
- Test machines are configured with `AutoDestroy: true`
- They should auto-destroy after running the curl command
- But there's a 5-minute timeout for DESTROYED state
- Should return error, not hang indefinitely

### 3. Network Routing Issue
- Test machine can't reach canary machine's private IP
- Curl hangs indefinitely (no timeout in curl command)
- Test machine never exits
- **This could explain the indefinite hang!**

### 4. Private IP Not Populated
- Canary machine's `PrivateIP` field is empty/null
- Test machine gets `FLY_TEST_MACHINE_IP=""`
- Curl tries to connect to invalid address
- Hangs or fails in unexpected way

## Most Likely Cause: Curl Has No Timeout

The test machine runs:
```bash
curl http://[$FLY_TEST_MACHINE_IP]:80
```

If the curl command can't connect (network issue, wrong IP, firewall, etc.), it will hang until:
- The default curl timeout (which can be several minutes or infinite)
- The machine is killed externally

The test machine won't auto-destroy until the command exits.

## Recommended Fixes

### Short-term: Add Timeout to Curl
Modify the test to include a timeout:
```toml
[[http_service.machine_checks]]
    image = "curlimages/curl"
    entrypoint = ["/bin/sh", "-c"]
    command = ["curl --max-time 30 --connect-timeout 10 http://[$FLY_TEST_MACHINE_IP]:80"]
```

### Medium-term: Add Overall Timeout to Test Machine Execution
In `machinebasedtest.go`, add a context timeout around the entire test machine execution.

### Long-term: Investigate Why Curl Can't Connect in Canary Scenario
- Check if temporary canary machines have different network configuration
- Verify PrivateIP is populated correctly
- Check if machine-to-machine connectivity works in CI environment
- Look for differences between temporary canary machines and regular machines

## Reproduction Steps

Toreproduce locally:
```bash
cd test/preflight
# Enable the commented-out test
# Run just that test
go test -tags=integration -v -timeout=20m -run TestFlyDeploy_DeployMachinesCheckCanary .
```

## Related Code Locations

- `internal/command/deploy/machines_deploymachinesapp.go:260-320` - deployCanaryMachines()
- `internal/command/deploy/machinebasedtest.go:44-150` - runTestMachines()
- `internal/appconfig/machines.go:108-220` - ToTestMachineConfig()
- `test/preflight/fly_deploy_test.go:394-417` - The failing test

## Timeline

- 2025-12-17: Issue discovered during CI runs on use-buildkit branch
- 2025-12-17: Tests commented out to unblock CI
- Investigation documented in this file
