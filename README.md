# flyctl

flyctl is a command-line interface for fly.io

Note: Most installations of `flyctl` also alias `flyctl` to `fly` as a command name and this will become the default name in the future.
During the transition, note that where you see `flyctl` as a command it can be replaced with `fly`.

## Installation

## Using a Package Manager

#### [Homebrew](https://brew.sh) (macOS, Linux, WSL)

```bash
brew install flyctl
```
To upgrade to the latest version:

```bash
brew upgrade flyctl
```

## Install Script

Download `flyctl` and install into a local bin directory.

#### MacOS, Linux, WSL

Installing the latest version:

```bash
curl -L https://fly.io/install.sh | sh
```

Installing the latest pre-release version:

```bash
curl -L https://fly.io/install.sh | sh -s pre
```

Installing a specific version:

```bash
curl -L https://fly.io/install.sh | sh -s 0.0.200
```

#### Windows

Run the Powershell install script:

```
iwr https://fly.io/install.ps1 -useb | iex
```


## Downloading from GitHub

Download the appropriate version from the [Releases](https://github.com/superfly/flyctl/releases) page of the `flyctl` GitHub repository.

## Getting Started

1. Sign into your Fly account

```bash
fly auth login
```

2. List your apps

```bash
fly apps list
```

2. View app status

```bash
fly status -a {app-name}
```

## App Settings

`flyctl` will attempt to use the app name from a `fly.toml` file in the current directory. For example, if the current directory contains this file:


```bash
$ cat fly.toml
app: banana
```

`flyctl` will operate against the `banana` app unless overridden by the -a flag or other app name setting in the command line.

## Building on Windows

There is a simple Powershell script, `winbuild.ps1`, which will run the code generation for the help files, format them, and run a full build, leaving a new binary in the bin directory.

## Running from branches on your local machine

Run `scripts/build-dfly` to build a Docker image from the current branch. Then, use `scripts/dfly` to run it. This assumes you are already
authenticated to Fly in your local environment.

## Cutting a release

If you have write access to this repo, you can ship a prerelease or full release with:

`scripts/bump_version.sh prerel`

or

`scripts/bump_version.sh`

## Running preflight tests

A preflight suite of integration tests is located under the test/preflight/ directory. It uses a flyctl binary and runs real user scenarios, including deploying apps and dbs, and validates expected behavior.

**Warning**: Real apps will be deployed that cost real money. The test fixture does its best to destroy resources it creates, but sometimes it may fail to delete a resource.

The easiest way to run the preflight tests is:

Copy `.direnv/preflight-example` to `.direnv/preflight` and edit following these guidelines:

* Grab your auth token from `~/.fly/config.yml`
* Do not use your "personal" org, create an new org (i.e. `flyctl-tests-YOURNAME`)
* Set 2 regions, ideally not your closest region because it leads
  to false positives when --region or primary region handling is buggy.
	Run `fly platform regions` for valid ids.

Finally run the tests:

	make preflight-test

That builds a flyctl binary (just like running `make`), then runs the preflight tests against that binary.

To run a single test:

```
make preflight-test T=TestAppsV2Example
```

Oh, add more preflight tests at `tests/preflight/*`
