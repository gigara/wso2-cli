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
        gateway:
          url: https://cloud-obs-proxy.obs.dp.dev.cloud.wso2.com/wso2cloud-obs
```

The gateway is WSO2 Cloud's observability proxy, which build logs are read
from. It is presented the same exchanged token as the API: it checks the
token's signature and organization claims, and no audience.

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
| `wso2 intg build logs <component> <build> [--tail <n>]` | Shows a build's log, or its last `n` lines, read the way the console reads it: a log query for the build's workflow run, sent directly to WSO2 Cloud's observability proxy, which the context records as the product's gateway. The search is bounded to the build's own run. At most 1000 lines; a build with no output yet says so. A context with no gateway is refused with `intg.observability_not_recorded`. |
| `wso2 intg environment list` | Lists the organization's environments: ID, display name, whether it is production, and data plane. Every command that takes an environment accepts its ID or its display name, and refuses an unknown one naming the IDs. |
| `wso2 intg deployment list <component>` | Lists what a component runs in every environment: status, release, build, commit and when it was deployed, or `not deployed`. |
| `wso2 intg deployment show <component> --env <environment>` | Shows what a component runs in one environment, with its schedule when it is an automation. |
| `wso2 intg component deploy <component> --env <environment> [--build <build>] [--project <project>]` | Deploys the component's current image, or a succeeded build's, to the environment where the project's deployment pipeline starts; other environments are refused with `intg.not_pipeline_entry`, naming the deploy and the promotion to run instead. A build that did not succeed or has no image is refused with `intg.build_not_deployable`, because the API would silently deploy the current image instead. `--project` defaults to the one the component listing reports. |
| `wso2 intg component promote <component> --from <environment> --to <environment> [--project <project>]` | Promotes the release running in one environment to another along the project's pipeline, looking up the release and the pipeline itself. Nothing deployed in `--from` is `intg.not_deployed`; a hop the pipeline refuses is `intg.promotion_refused`. |
| `wso2 intg component stop <component> --env <environment>` | Stops the component in one environment, keeping its deployment. |
| `wso2 intg component redeploy <component> --env <environment>` | Restarts the release deployed in one environment, or starts it again after a stop. |

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
