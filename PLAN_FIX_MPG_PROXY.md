# Plan: Decouple `fly mpg proxy` from database credentials

## Problem

`fly mpg proxy` currently uses the same `GetMpgProxyParams` helper as
`fly mpg connect`. That helper resolves and validates database credentials before
it builds the network proxy parameters.

This coupling produces a misleading failure for an otherwise-ready cluster when
the default database user or its stored password is missing:

```text
Error: cluster is still initializing, wait a bit more
```

For MPGv2, ui-ex returns `credentials.status = "initializing"` when pg-admin
cannot find credentials for the default `fly-user`. For MPGv1, ui-ex returns the
same status when neither the legacy-user secret nor the default-user secret is
available. flyctl treats that credential status as cluster readiness in both
versions.

The raw proxy does not use database credentials. It only needs:

- the cluster's direct IP;
- the organization slug used for the WireGuard tunnel;
- the local bind address and port;
- the Fly agent dialer.

`RunProxy` currently discards both the returned cluster credentials and the
cluster itself after calling `GetMpgProxyParams`.

## Goals

- Allow `fly mpg proxy` to start when a cluster has a direct IP, even if its
  default database credentials are missing or report `initializing`.
- Preserve all existing credential selection and validation for
  `fly mpg connect`, including explicit `--username` handling.
- Apply the same behavior to MPGv1 and MPGv2.
- Preserve the existing missing-direct-IP error and WireGuard setup behavior.
- Add regression coverage for the distinction between network proxy readiness
  and credential readiness.

## Non-goals

- Do not change ui-ex response shapes or stop the cluster endpoints from
  attempting their existing credential lookup.
- Do not recreate missing database users or passwords.
- Do not change `fly mpg connect`, attach, create, or credential-management UX.
- Do not broaden organization or cluster authorization. The existing cluster
  lookup and organization-scoped WireGuard tunnel remain the authorization
  boundary.
- Do not make `fly mpg proxy` depend on the cluster's displayed status. A raw
  tunnel can be useful during maintenance or another transitional state, and
  the direct IP is the actual network prerequisite.

## Current call flow

The v1 and v2 implementations are effectively identical:

```text
RunProxy
  -> GetMpgProxyParams(username = "")
       -> fetch cluster response
       -> read embedded default credentials
       -> reject credentials.status == "initializing"
       -> reject empty/error password
       -> validate direct IP
       -> establish agent and WireGuard dialer
       -> build proxy.ConnectParams
  -> proxy.Connect
```

`RunConnect` calls the same helper and genuinely needs the credentials returned
by it to construct the `psql` connection URL.

No other flyctl callers use `GetMpgProxyParams`; the only callers are `RunProxy`
and `RunConnect` in each version package.

## Proposed design

Split cluster/network setup from credential resolution in both
`internal/command/mpg/v1` and `internal/command/mpg/v2`.

Use two explicit paths:

```text
RunProxy
  -> fetch cluster
  -> validate direct IP
  -> build network proxy parameters
  -> proxy.Connect

RunConnect
  -> fetch cluster
  -> resolve default or requested-user credentials
  -> preserve current credential validation
  -> validate direct IP
  -> build network proxy parameters
  -> proxy.Start
  -> launch psql
```

Within each version package:

1. Extract cluster retrieval into a small helper that preserves the current
   version-specific client call and error wrapping.
2. Extract credential selection and validation into a connect-only helper.
   Preserve the current behavior exactly:
   - default credentials come from the cluster response;
   - an explicit username calls `GetUserCredentials`;
   - `initializing`, error, and empty-password cases retain their current
     errors;
   - the default database continues to come from the cluster response.
3. Extract direct-IP validation and `proxy.ConnectParams` construction into a
   network helper. Keep agent establishment after all connect-specific
   credential validation so `fly mpg connect` does not open a tunnel before a
   credential error is reported.
4. Have `RunProxy` call only the cluster and network helpers. It must not inspect
   `response.Credentials`.
5. Have `RunConnect` call the cluster, credential, and network helpers in that
   order.

Prefer descriptive names such as `getCluster`, `resolveConnectCredentials`, and
`buildProxyParams`. If exported helpers remain useful for tests, use
`GetMpgConnectParams` for the credential-bearing path and reserve
`GetMpgProxyParams` for the credential-free path. Avoid a boolean such as
`skipCredentials`; separate functions make it harder to accidentally restore
the coupling later.

Although v1 and v2 contain duplicated implementations, keep this change within
their existing packages. Introducing a generic cross-version abstraction would
substantially widen the patch because the response, cluster, credential, and
client types differ. A later cleanup can consolidate them after the behavior is
covered by tests.

## MPGv1 compatibility notes

MPGv1 does not require credentials to build the proxy target:

- `Data.IpAssignments.Direct` is populated from the managed-service IP
  assignment.
- Default credentials are populated independently from a Kubernetes Secret,
  with a legacy-user-secret fallback.
- The Fly agent and organization tunnel do not consume the database username,
  password, database name, or connection URI.

Therefore, ignoring the embedded credential status for `mpg proxy` does not
remove a network dependency or an authorization check. The MPGv1 cluster API
will still perform its current credential lookup while rendering the response;
flyctl will simply stop treating the result as a prerequisite for a raw tunnel.

## Implementation steps

1. Refactor `internal/command/mpg/v1/run_proxy.go`:
   - separate cluster retrieval, credential resolution, and proxy construction;
   - make `RunProxy` credential-free;
   - retain the current direct-IP and agent/tunnel errors.
2. Update `internal/command/mpg/v1/run_connect.go` to use the connect-specific
   credential path without changing user/database selection or `psql` startup.
3. Apply the equivalent refactor to
   `internal/command/mpg/v2/run_proxy.go` and update
   `internal/command/mpg/v2/run_connect.go`.
4. Run `gofmt` on all changed Go files.
5. Add focused tests for both version packages. Use pure helpers for response
   interpretation where possible so tests do not need a real Fly agent or
   WireGuard tunnel.
6. Run the scoped tests, then the broader MPG command tests.

## Test cases

Add equivalent v1 and v2 coverage for:

### Proxy path

- A cluster response with a direct IP and
  `credentials.status = "initializing"` is accepted as a proxy target.
- A cluster response with a direct IP and an empty password is accepted as a
  proxy target.
- A missing direct IP still returns `error getting cluster IP`.
- Proxy parameter construction retains the requested local port, remote port
  `5432`, bind address, organization slug, and direct IP.

### Connect path

- Default credentials with `status = "initializing"` still return
  `cluster is still initializing, wait a bit more`.
- Default credentials with `status = "error"` or an empty password still return
  `error getting cluster password`.
- Explicit-user credentials with an empty password still return
  `error getting user password`.
- Explicit usernames still use `GetUserCredentials`, while the default database
  continues to come from the cluster response.

### Command routing

- Existing version detection continues to route v1 clusters to `cmdv1.RunProxy`
  and v2 clusters to `cmdv2.RunProxy`.

If direct testing of `RunProxy` would require starting a real agent or blocking
inside `proxy.Connect`, test the extracted pure target/credential helpers and
keep the command functions as small orchestration wrappers. Do not introduce a
large mocking framework solely for this change.

## Verification commands

From the flyctl repository root:

```sh
gofmt -w internal/command/mpg/v1/run_proxy.go \
  internal/command/mpg/v1/run_connect.go \
  internal/command/mpg/v2/run_proxy.go \
  internal/command/mpg/v2/run_connect.go \
  internal/command/mpg/v1/run_proxy_test.go \
  internal/command/mpg/v2/run_proxy_test.go

go test ./internal/command/mpg/v1 ./internal/command/mpg/v2
go test ./internal/command/mpg/...
```

Adjust the `gofmt` file list if tests are added to differently named files.

## Acceptance criteria

- `fly mpg proxy` no longer emits the initializing/password errors solely
  because default credentials are unavailable when a direct IP exists.
- `fly mpg connect` retains its current credential errors and explicit-user
  behavior.
- MPGv1 and MPGv2 behave consistently.
- Missing direct IPs still fail before tunnel establishment.
- No ui-ex change is required.
- All scoped MPG tests pass.

