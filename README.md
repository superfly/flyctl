# flyctl

flyctl is a command line interface for fly.io

## Installation

## Using a Package Manager

#### [Homebrew](https://brew.sh) (macOS, Linux, WSL)

```bash
brew install superfly/tap/flyctl
```

## Install Script

Download `flyctl` and install into 

Installing the latest version:

```bash
curl https://get.fly.io/flyctl.sh | sh
```

Installing a specific version:

```bash
curl https://get.fly.io/flyctl.sh | sh -s v0.0.1
```

Install into a bin directory other than `/usr/local/bin`:
```bash
BIN_DIR=~/.bin curl https://get.fly.io/flyctl.sh | sh
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
flyctl apps
```

2. Interact with an app

```bash
flyctl status -a {app-name}
```

## App Settings

`flyctl` will attempt to use the app name from a `fly.toml` file in the current directory. For example, if the current directory contains this file:


```bash
$ cat fly.toml
app: banana
```

`flyctl` will operate against the `banana` app
