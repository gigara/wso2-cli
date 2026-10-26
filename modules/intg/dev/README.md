# Trying the intg module against WSO2 Cloud dev

This directory is a local test kit, not part of the module. It builds the shell
and this module from the checkout and records one context, `wso2cloud-dev`, for
the WSO2 Cloud development environment.

## Before you start

- Go 1.26.9 or later on `PATH`.
- A desktop session with a browser and an OS keychain. The macOS Keychain works;
  on Linux, a running secret service such as GNOME Keyring.
- Network access to `platform-idp-development.gateway.dev.cloud.wso2.com`, which
  may need the WSO2 network or VPN.
- A WSO2 Cloud dev account in an organization that has the Integration Platform.

## Run it

```sh
modules/intg/dev/setup.sh
export WSO2_HOME=$HOME/.wso2-intg-test
./bin/ws intg status
./bin/ws login                       # sign in and pick your organization
./bin/ws intg project list --verbose
./bin/ws intg project list -o json
```

The state lives in `~/.wso2-intg-test`, apart from your own `~/.wso2`. Re-run
`setup.sh` after changing code; it rebuilds and reinstalls, keeps the context
and any login it holds, and applies the context again only when the kit file
changes. Remove that directory to start over.

## How the context reaches the API

The login runs at the WSO2 Cloud ThunderID with the public `WSO2_CLI` client and
names no resource, because the context's only product is reached by exchange.
Each `ws intg` command exchanges the login token (RFC 8693) for one bound to the
Integration Platform API, and the shell checks that token's audience before the
module receives it. An earlier version of this kit also recorded a
`wso2cloud-dev-direct` workaround context; `setup.sh` deletes it.

## Seeing the tokens (temporary)

`WSO2_DEBUG_TOKENS` makes the `--verbose` log show the tokens an exchange runs
on: the login access token and the token it is exchanged for.

```sh
WSO2_DEBUG_TOKENS=claims ./bin/ws intg project list --verbose   # decoded header and claims only
WSO2_DEBUG_TOKENS=raw ./bin/ws intg project list --verbose      # also the tokens themselves
```

`claims` is usually enough: issuer, audience, scopes, `client_id`, the `ou*`
claims and expiry. A `raw` token works as a bearer credential until it expires,
so don't paste one into a chat or a ticket. Both modes need `--verbose`, and
nothing is logged with the variable unset. This aid is temporary and will be
removed once the deployment is understood.

## What to send back if it fails

The `error:` line and its code, such as `auth.narrowing_unavailable` or
`intg.not_authorized`, and the `--verbose` log from standard error, which has
secrets redacted. Tokens never appear on any output.
