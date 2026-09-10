# Credential profiles

Profiles let one machine hold credentials for several Fly.io accounts at once and
pick between them per shell, per project or per command, instead of running
`fly auth logout` and `fly auth login` to switch.

A profile is a complete flyctl config directory, not just a token, so each one
carries its own access token, metrics token, WireGuard peer state, agent socket and
lock files. `$HOME` is untouched, so Docker and SSH credentials keep working.

The `default` profile *is* `~/.fly`. An installation that never runs `fly profile`
behaves exactly as before.

## Usage

```sh
fly profile add work            # log an account into a new, isolated profile
fly profile add client-a

cd ~/projects/acme
fly profile link client-a       # writes .fly-profile for this directory tree

fly deploy                      # uses client-a, with no switching and no flags
```

## Resolution order

Highest precedence first:

| # | Rule | Scope |
|---|------|-------|
| 1 | `FLY_CONFIG_DIR` | pins a config directory outright, bypassing profiles |
| 2 | `--profile <name>` | a single command |
| 3 | `FLY_PROFILE=<name>` | a shell |
| 4 | `.fly-profile`, nearest at or above the working directory | a directory tree |
| 5 | `fly profile use <name>` | the machine |
| 6 | `default` | `~/.fly` |

`FLY_ACCESS_TOKEN` and `FLY_API_TOKEN` continue to override the resolved config, as
before, so existing CI setups are unaffected.

A profile named by any of these rules that does not exist is an error rather than a
silent fallback, since reaching a different account than the one asked for is worse
than not running. The `fly profile` commands are exempt so that a dangling reference
can be repaired; `fly profile show` reports what is wrong.

## Commands

```
fly profile list [--refresh]        list profiles, accounts and credential status
fly profile add <name> [--token T] [--use]
fly profile use <name>              switch the machine-wide profile
fly profile link <name>             pin the current directory tree
fly profile show                    which profile is in effect, and why
fly profile rename <old> <new>
fly profile remove <name>
```

`fly profile list` shows the account name from a local cache written at login;
`--refresh` re-checks each profile against the API, which also surfaces expired or
revoked tokens.

## Layout

```
~/.fly/
├── config.yml                <- the "default" profile
├── active_profile            <- written by `fly profile use`
└── profiles/
    └── work/
        ├── config.yml        <- its own token and WireGuard state
        └── profile.yml       <- cached account email
```

`FLY_PROFILE_HOME` relocates the store, which is mainly useful for testing.

## Scripted setup

```sh
fly profile add prod --token "$FLY_PROD_TOKEN"
fly profile add staging --token "$FLY_STAGING_TOKEN"
```
