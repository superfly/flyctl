# flyctl

flyctl is a command-line interface for fly.io.

Note: Most installations of `flyctl` also alias `flyctl` to `fly` as a command name, and this will become the default name in the future. During the transition, note that where you see `flyctl` as a command, it can be replaced with `fly`.

## Installation

### Using a Package Manager

#### [Homebrew](https://brew.sh) (macOS, Linux, WSL)

```bash
brew install flyctl
```

To upgrade to the latest version:

```bash
brew upgrade flyctl
```

### Install Script

Download `flyctl` and install it into a local bin directory.

#### macOS, Linux, WSL

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

Run the PowerShell install script:

```powershell
iwr https://fly.io/install.ps1 -useb | iex
```

### Downloading from GitHub

Download the appropriate version from the [Releases](https://github.com/superfly/flyctl/releases) page of the `flyctl` GitHub repository.

## Getting Started

1. Sign into your Fly account:

    ```bash
    fly auth login
    ```

2. List your apps:

    ```bash
    fly apps list
    ```

3. View app status:

    ```bash
    fly status -a {app-name}
    ```

## App Settings

`flyctl` will attempt to use the app name from a `fly.toml` file in the current directory. For example, if the current directory contains this file:

```bash
$ cat fly.toml
app: banana
```

`flyctl` will operate against the `banana` app unless overridden by the `-a` flag or another app name setting in the command line.

## Releases

`flyctl` is automatically released at 3 PM Eastern Standard Time, Monday - Thursday. If needed, you can bump a release by running `./scripts/bump_version.sh`.

## Building on Windows

There is a simple PowerShell script, `winbuild.ps1`, which will run the code generation for the help files, format them, and run a full build, leaving a new binary in the bin directory.

## Running from Branches on Your Local Machine

Run `scripts/build-dfly` to build a Docker image from the current branch. Then, use `scripts/dfly` to run it. This assumes you are already authenticated to Fly in your local environment.

## Contributing Guide

See [CONTRIBUTING.md](./CONTRIBUTING.md)
