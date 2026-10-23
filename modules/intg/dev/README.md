# Trying the intg module against WSO2 Cloud dev

This directory is a local test kit, not part of the module. It builds the shell
and this module from the checkout and records two contexts for the WSO2 Cloud
development environment.

## Before you start

- Go 1.26.9 or later on `PATH`.
- A desktop session with a browser and an OS keychain. The macOS Keychain works;
  on Linux, a running secret service such as GNOME Keyring.
- Network access to `platform-idp-development.gateway.dev.cloud.wso2.com`, which
  may need the WSO2 network or VPN.
- A WSO2 Cloud dev account in an organization that has the Integration Platform.

## Run it

```sh
modules/intg/dev/setup.sh            # selects wso2cloud-dev-direct
export WSO2_HOME=$HOME/.wso2-intg-test
./bin/ws intg status
./bin/ws login                       # sign in and pick your organization
./bin/ws intg project list --verbose
./bin/ws intg project list -o json
```

The state lives in `~/.wso2-intg-test`, apart from your own `~/.wso2`. Re-run
`setup.sh` after changing code; it rebuilds and reinstalls, and keeps the
contexts and any login they hold. Remove that directory to start over.

## The two contexts

| Context | How it reaches the API | Status |
| --- | --- | --- |
| `wso2cloud-dev` | The intended design: the login token is exchanged per command for one bound to the Integration Platform API. | `ws login` refuses with `auth.product_not_configured` until the shell lets a ThunderID login bind to no direct product. |
| `wso2cloud-dev-direct` | A workaround: the login itself names the API as its resource, and no exchange runs. | Gets past that check and starts the browser login. Untested beyond that. |

Switch with `./bin/ws context use <name>`.

## What to send back if it fails

The `error:` line and its code, such as `auth.narrowing_unavailable` or
`intg.not_authorized`, and the `--verbose` log from standard error, which has
secrets redacted. Tokens never appear on any output.
