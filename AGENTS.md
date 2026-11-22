# AGENTS.md

`flyctl` is the Fly.io CLI, written in Go. The Go version is in `go.mod`. Commands live under
`internal/command/`. [CONTRIBUTING.md](CONTRIBUTING.md) has the full development flow and
[building.md](building.md) covers versioning and releases.

## Build and run

```sh
make build          # runs go generate, then builds bin/flyctl
./bin/flyctl version
go run . <command>  # build and run in one step (slower startup)
```

## Test

```sh
make test                   # unit tests: go test ./...
make test T=TestName        # one test (passed to --run)
```

Unit tests sit next to the code they test (`foo_test.go` beside `foo.go`). CI runs `make test`
on Linux, macOS and Windows.

Preflight tests in `test/preflight` (`make preflight-test`) create real apps and Machines on
production and cost money. They need a test org and token in `.direnv/preflight` (copy
`.direnv/preflight-example`). Don't run them unless asked; CI runs them on pull requests.

## Generated code

```sh
go generate ./...   # help text and the GraphQL client (genqlient)
```

CI runs `go generate ./... && git diff --exit-code`, so commit whatever it changes.
`gql/schema.graphql` comes from Fly.io's internal `web` repo; don't edit it by hand.

## Lint

```sh
make lint   # golangci-lint run ./...
make pre    # pre-commit run --all-files (whitespace, go mod tidy, golangci-lint)
```

## Pull requests

- Fill in `.github/pull_request_template.md`, including the Documentation checkbox.
- PRs are squash merged, so the PR title becomes the commit message: make it descriptive.
- Keep commits small and say why the change is needed.
