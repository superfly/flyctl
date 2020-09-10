# flyctl

flyctl is a command-line interface for fly.io

Note: Most installations of `flyctl` also alias `flyctl` to `fly` as a command name and this will become the default name in the future.
During the transition, note that where you see `flyctl` as a command it can be replaced with `fly`.

## Installation

## Using a Package Manager

#### [Homebrew](https://brew.sh) (macOS, Linux, WSL)

```bash
brew install superfly/tap/flyctl
```
To upgrade to the latest version:

```bash
brew upgrade flyctl
```

## Install Script

Download `flyctl` and install into 

Installing the latest version:

```bash
curl -L https://fly.io/install.sh | sh
```

Installing a specific version:

```bash
curl -L https://fly.io/install.sh | sh -s v0.0.1
```

## Downloading from GitHub

Download the appropriate version from the [Releases](https://github.com/superfly/flyctl/releases) page of the `flyctl` GitHub repository.

## Getting Started

1. Sign into your fly account

```bash
flyctl auth login
```

2. List your apps

```bash
flyctl apps list
```

2. View app status

```bash
flyctl status -a {app-name}
```

## App Settings

`flyctl` will attempt to use the app name from a `fly.toml` file in the current directory. For example, if the current directory contains this file:


```bash
$ cat fly.toml
app: banana
```

`flyctl` will operate against the `banana` app unless overridden by the -a flag or other app name setting in the command line.
