# The intg product module

The `intg` module manages integrations on the WSO2 Cloud Integration Platform,
through the platform's own API (`ipaas-service`).

It is a WSO2 CLI product module. It is a separate program: the `ws` shell
resolves it from the managed module store and launches it, and the two speak the
module contract over the module's standard input and output.

It lives in this repository and is released by its own tag, independently of the
shell and of every other module:

```sh
git tag intg/v0.1.0-rc.1
git push origin intg/v0.1.0-rc.1
```

That tag builds the module for every supported platform, publishes the archives,
and regenerates the module catalog a user installs from. A release is refused if
no shell that exists can launch it.

## Working on it

```sh
# From the repository root.
make build-module NAMESPACE=intg
make test-module NAMESPACE=intg
make install-module NAMESPACE=intg   # then ./bin/ws intg status
```

The workspace composes this module with the SDK from source, so a change to the
SDK reaches it without a release. The `go.mod` here requires the published SDK
at the version every other module in this repository requires, and carries no
`replace` directive.

## How it reaches the platform

The shell signs in at the WSO2 Cloud ThunderID with the public `WSO2_CLI`
client. For each command it exchanges that login token at the same ThunderID
(RFC 8693) for one bound to the Integration Platform API, whose resource-server
identifier is the API's own URL. The module asks for the logical audience
`integration-management` and receives only that short-lived token. The API
reads the organization from the token's `ouId`, `ouHandle` and `ouName` claims,
so no command takes an organization.

`module.json` declares the product as reached by exchange, so recording it
needs only its URL, and the audience defaults to that URL. WSO2 Cloud's
ThunderID names its issuer `platform-idp` rather than its URL, so the context
declares that as `issuerId`:

```yaml
contexts:
  - name: wso2cloud-dev
    type: cloud
    login:
      kind: oauth-browser
      provider: thunder
      issuer: https://platform-idp-development.gateway.dev.cloud.wso2.com
      issuerId: platform-idp
      clientId: WSO2_CLI
    products:
      intg:
        url: https://development-wso2cloud.gateway.dev.cloud.wso2.com/ipaas-service-ipaas-api-endpoint
```

```sh
ws context apply -f wso2cloud-dev.yaml --use wso2cloud-dev
ws login
ws intg project list
```

The login names no resource, because the context's only product is reached by
exchange; each command then exchanges the login token for the product's own
resource. `dev/` holds a kit that builds and records all of this locally.

## Commands

| Command | Does |
| --- | --- |
| `wso2 intg status` | Reports this module's own status and what to run first. Calls nothing. |
| `wso2 intg project list` | Lists the projects in the organization the token names, with each project's ID, deployment pipeline, linked repository and state. The API returns one page of at most 20 projects and no continuation cursor. |
| `wso2 intg project show <project>` | Shows one project: ID, name, description, pipeline, repository with its provider, branch, path, creation time and state. A 404 names `wso2 intg project list`. |
| `wso2 intg project create <name> [--display-name <text>] [--description <text>] [--pipeline <pipeline>] [--repo <url> --branch <branch> [--path <dir>]]` | Creates a project, sending only the options given. `--repo` takes a GitHub or Bitbucket URL; a partial link is refused, because the API drops one without saying so. A name held by a different project is `intg.project_exists`. The API's 503, a project created but not fully set up, is `intg.project_incomplete`, and running the same command again finishes it. |
| `wso2 intg project delete <project> --yes` | Deletes a project and everything in it. Without `--yes` it calls nothing. Deletion continues in the background, and the project is listed with state `deleting` until it is gone. |
| `wso2 intg component list [--project <project>]` | Lists a project's components, or every project's when none is named, with ID, project, type, runtime, status, last update and state. Type and runtime use the words `component create` takes. Each project lists at most 100 components, with no continuation cursor. |
| `wso2 intg component show <component> [--project <project>]` | Shows one component and its source repository. The API has no read of one component, so it is found in the project's listing, by ID first and then by display name; a display name two components share is refused, naming their IDs. Without `--project` every project is searched. |
| `wso2 intg component create <name> --project <project> --type <type> --runtime <runtime> --repo <url> --branch <branch> [--path <dir>] [--display-name <text>] [--description <text>] [--auto-deploy]` | Creates a component that builds from a public git repository and starts its first build, sending the body the platform's own `create_integration` MCP tool sends. Types: `integration-as-api`, `automation`, `event-integration`, `file-integration`, `mcp-server`. Runtimes: `wso2-integrator`, `wso2-integrator-mi`, `dockerfile`. A taken name is suffixed by the platform, and the result reports the ID it created and any warning. |
| `wso2 intg component delete <component> --yes [--project <project>]` | Deletes a component. Without `--yes` it calls nothing. The platform resolves the project when none is named. Deletion continues in the background. |
| `wso2 intg build run <component> [--commit <sha>]` | Starts a build of the component's branch head, or of a full 40-character commit SHA, since a build cannot fetch a short one. The API does not name the build it started, so the result points at `build list`, which lists the newest first. |
| `wso2 intg build list <component>` | Lists a component's recent builds, newest first: name, status, trigger, the first 12 characters of the commit, start and completion. The API returns at most 20, with no continuation cursor. |
| `wso2 intg build show <component> <build>` | Shows one build: status, trigger, full commit, image, times and each step's phase. The next step follows the status: the logs of a failed build, or `build show` again for one still running. The API answers an unknown build name with a 500, so that refusal also points at `build list`. |
| `wso2 intg build logs <component> <build> [--tail <n>]` | Shows a build's log, or its last `n` lines, read the way the console reads it: a log query for the build's workflow run through the API's `/observability` pass-through to WSO2 Cloud's observability proxy, bounded to the build's own run. The API's own build-log route is not used, because it asks the proxy for a path the proxy does not serve. At most 1000 lines; a build with no output yet says so. |

Every failure keeps the API's own words. A missing product URL is
`intg.product_not_recorded`; 401 and 403 are `intg.not_authorized`; 402 is
`intg.plan_limit`; 404 is `intg.not_found`; other refusals are
`intg.call_failed`; no answer at all is `intg.deployment_unreachable`.

## What this module must not do

It must not import anything under the shell's `internal` tree, and it must not
print to standard output: that stream carries protocol frames, and anything
written there directly corrupts them. Diagnostics go to standard error. Both
rules are asserted for every module under `modules/`, including this one, by the
tests in `internal/boundaries`.
