# Preflight Test CI Setup

## Required GitHub Secrets

The preflight tests require the following GitHub repository secrets to be configured:

### `FLYCTL_PREFLIGHT_CI_USER_TOKEN` (Required for deploy token tests)

This must be a **user token** (not a limited access token) with permissions to:
- Create apps in the `flyctl-ci-preflight` organization
- Create deploy tokens (requires user-level permissions)
- Manage machines, volumes, and other resources

**How to create:**
1. Log in to Fly.io with a user account that has access to the `flyctl-ci-preflight` org
2. Run: `flyctl auth token`
3. Copy the token (it should NOT end with `@tokens.fly.io`)
4. Add it to GitHub Secrets as `FLYCTL_PREFLIGHT_CI_USER_TOKEN`

### `FLYCTL_PREFLIGHT_CI_FLY_API_TOKEN` (Fallback)

This is the fallback token used when `FLYCTL_PREFLIGHT_CI_USER_TOKEN` is not available. It can be either a user token or a limited access token.

**Current behavior:**
- If `FLYCTL_PREFLIGHT_CI_USER_TOKEN` exists, it will be used (preferred)
- If not, falls back to `FLYCTL_PREFLIGHT_CI_FLY_API_TOKEN`
- Deploy token tests will fail if neither is a user token

## Why Two Tokens?

We use two separate tokens for security reasons:

1. **Most tests** can run with limited access tokens (more secure, limited blast radius)
2. **Deploy token tests** require user tokens (can create other tokens)

By having both, we can use the least privileged token for most operations while still supporting the full test suite.

## Verifying Token Type

To check if a token is a user token vs limited access token:

```bash
# Set the token
export FLY_API_TOKEN="your-token-here"

# Check the user
flyctl auth whoami
```

**User token output:** `user@example.com` or `uuid@some-domain.com`
**Limited access token output:** `uuid@tokens.fly.io` (ends with `@tokens.fly.io`)

Deploy token tests **require** a token that does NOT end with `@tokens.fly.io`.
