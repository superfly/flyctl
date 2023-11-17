# Development flow


## Building

To build `flyctl`, all you need to do is to run `make build` from the root directory. This will build a binary in the `bin/` directory. Alternatively, you can run `go build -o <EXE_NAME> .`

To run `flyctl`, you can just run the binary you built using `make build`: `./bin/flyctl`. So for example, to update a machine, you can run `go run . m update -a <app_name> <machine_id>`. Alternatively, you can build and run in the same command by running `go run .`, followed by whatever sub-command you want to run. Just note that this will have a slower startup.


## Testing

We have two different kinds of tests in `flyctl`, unit tests and integration tests (preflight). It's recommend to write a test for any features added or bug fixes, in order to prevent regressions in the future.


### Integration tests

Unit tests are stored in individual files next to the functionality they're testing. For example

`internal/command/secrets/parser_test.go`
is a test for the secrets parsing code

`internal/command/secrets/parser_test.go`.
You can run these tests by running `make test` from the root directory.


### Preflight

The integration tests, called preflight, are different. They exist to test flyctl functionality on production infra, in order to make sure that entire commands and workflows don't break. Those are located in the `test/preflight` directory.

For outside contributors, **please be warned that running preflight tests creates real apps and machines and will cost real money**. We already run preflight by default on all pull requests, so we recommend just opening up a draft PR instead. If you work at Fly and want to work on preflight tests, go ahead and continue reading.

Before running any preflight test, you must first set some specific environment variables. It's recommended to set them up using [direnv](https://direnv.net/docs/installation.html). First, copy the `.direnv/preflight-example` file to `.direnv/preflight`. Next, modify `FLY_PREFLIGHT_TEST_FLY_ORG` to an organization you make specifically for testing. Don't use your `personal` org. Modify `FLY_PREFLIGHT_TEST_FLY_REGIONS` to have two regions, ideally ones not the closest ones. For example, `"iad sin"`. Finally, set `FLY_PREFLIGHT_TEST_ACCESS_TOKEN` to whatever `fly auth token` outputs.

To run preflight tests, you can just run `make preflight-test`. If you want to run a specific preflight test, run `make preflight-test T=<test_name>`

If you're trying to decide whether to write a unit test, or an integration test for your change, I recommend just writing a preflight test. They're usually simpler to write, and there's a lot more examples of how to write them.



## Linting

With the trifecta of the development process nearly complete, let's talk about linting. The linter we run is [golangci-lint](https://golangci-lint.run/). It helps with finding potential bugs in programs, as well as helping you follow standard go conventions. To run it locally, just run `golangci-lint --path-prefix=. run`. If you'd like to run all of our [pre-commit lints](https://pre-commit.com/), then run `pre-commit run --all-files`

# Generating the GraphQL Schema

As of writing this, we host our GraphQL schema on `web`, an internal repo that hosts our GQL based API. Unfortunately, that means that outside contributors can't updated the GraphQL schema used by `flyctl`. While there isn't much of a reason why you may want to do so, we're working on automating update the GQL schema in `flyctl`.

Updating the GraphQL schema from web is a manual process at the moment. To do so, `cd` into `web/`, and run `bundle exec rails graphql:schema:idl && cp ./schema.graphql ../flyctl/gql/schema.graphql`, assuming that `flyctl/` is in the same directory as `web/`


# Cutting a release

If you have write access to this repo, you can ship a prerelease with:

`scripts/bump_version.sh prerel`

or a full release with:

`scripts/bump_version.sh`


# Committing to flyctl

When committing to `flyctl`, there are a few important things to keep in mind:

-   Keep commits small and focused, it helps greatly when reviewing larger PRs
-   Make sure to use descriptive messages in your commit messages, it also helps future people understand why a change was made.
-   PRs are squash merged, so please make sure to use descriptive titles


## Examples

[This is a bad example of a commit](https://github.com/superfly/flyctl/pull/1809/commits/6f167c858dbd7ae1324632dda9e29072ddde8ad7), it has a large diff and no explanation as to why this change is being made. [This is a great one](https://github.com/superfly/flyctl/commit/2636f47fe91cbe37018926cb0d7d2227a6887086), since it's a small commit, and it's reasoning as well as the context behind the change. Good commit messages also help contributors in the future to understand *why* we did something a certain way.


# Further Go reading

Go is a weird language full of a million different pitfalls. If you haven't already, I strongly recommend reading through these articles:

-   <https://go.dev/doc/effective_go> (just a generally great resource)
-   <https://go.dev/blog/go1.13-errors> (error wrapping specifically is useful for a lot of the functionality we use)
