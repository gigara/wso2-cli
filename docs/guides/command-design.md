# Design a CLI command

Use this guide when adding or reviewing a shell command or a product module's
command tree. It applies to new commands. Existing command names stay stable
unless a separate migration changes them.

The release executable is `ws`. Most repository documentation writes `wso2`
because the name is selected at build time. Substitute the name of the binary
you are running; the command structure is the same.

## Command shape

```text
ws <product-namespace> [<resource>] <action> [<arguments>] [flags]
```

- The shell owns shared commands such as `context`, `login`, `org`, and
  `product`. Put a command there only when its behavior applies across
  products. A product module owns exactly one top-level namespace, such as
  `apim`, `iam`, `am`, or `intg`. Do not use a shell command name as a
  product namespace.
- Under a product namespace, group operations on the same kind of resource
  under one noun. Put the action at the end of the command path. Omit the
  resource group when the namespace already names the primary resource, or
  when the action applies to the product as a whole, such as
  `ws <namespace> status`.
- Add another noun level only when it identifies a real child resource. A
  command path should describe the target without repeating the product name
  unnecessarily.

The `apim` module follows this shape. A resource group holds its actions, and
a child resource gets its own noun level under its parent:

```text
ws apim api list
ws apim api create --file <file> --project <project>
ws apim api deploy <api> --gateway-id <gateway>
ws apim api delete <api> --yes
ws apim gateway api list
ws apim gateway token create <gateway>
```

`ws product install apim` is a shell command because it installs the product
module; `ws apim ...` contains that module's product operations.

A short namespace such as `am` does not name a resource by itself, so the
resource it manages still gets a group, beside the other resources:

```text
ws am agent list
ws am agent show <agent>
ws am project list
```

These are naming examples, not commands currently supplied by an `am` module.

## The rules

Ten rules, each with an example that follows it and one that does not. ADR
0015 (one word per concept in the command surface) is the decision behind
most of them, and the domain glossary in [`CONTEXT.md`](../../CONTEXT.md)
supplies the project's terms.

### 1. Namespace, resource, action, in that order

| Do | Don't |
| --- | --- |
| `ws apim api list` | `ws apim list-apis` |
| `ws iam resource-server create <name>` | `ws iam create resource-server <name>` |
| `ws apim gateway token create <gateway>` | `ws apim create-gateway-token <gateway>` |

A child resource gets its own noun level under its parent. Skip the resource
noun only when the action applies to the whole product: `ws apim status`.

### 2. Singular nouns, always

| Do | Don't |
| --- | --- |
| `ws apim api list` | `ws apim apis list` |
| `ws iam user delete <user>` | `ws iam users delete <user>` |

The group stays singular for every action, including `list`. This matches
the shell's own `context`, `product`, and `org` families, so one rule covers
the whole command tree.

### 3. One verb per operation

| Operation | Verb | Not |
| --- | --- | --- |
| A collection | `list` | `ls`, `get-all` |
| One resource | `show <id>` | `get`, `describe`, `view` |
| A new resource | `create` | `add`, `new` |
| A change | `update` | `edit`, `set` |
| Removal | `delete <id> --yes` | `remove`, `rm` |
| A product operation | `deploy`, `invoke`, `publish` | A generic verb that hides what happens |

Use a product verb such as `deploy` when it names a distinct operation. Do not
introduce `get` beside `show`, or `remove` beside `delete`, for the same
operation in one command family.

### 4. Don't repeat the namespace

| Do | Don't |
| --- | --- |
| `ws am agent list` | `ws am am-agent list` |
| `ws intg component deploy` | `ws intg intg-component deploy` |

### 5. Flags: lowercase, hyphenated, the same name everywhere

| Do | Don't |
| --- | --- |
| `--file`, `-f` for an input document | `--filename` here, `--spec` there |
| `--project <id>` in every command that takes one | `--project-id` in one, `--proj` in another |
| `--gateway-id <id>` | `--gatewayId` |

Give a flag the same name and meaning wherever the same choice appears. Use
descriptive placeholders in angle brackets for required values and square
brackets for optional syntax: `<environment>`, `[<name>]`. Shell flags are
reserved: a module never declares `--output`, `--context`, `--no-input`,
`--verbose`, or `--help`. The shell reads them wherever they appear on a
product command line and never forwards them.

### 6. Secrets never on the command line

| Do | Don't |
| --- | --- |
| `--client-secret-variable WSO2_CLIENT_SECRET` | `--client-secret s3cr3t` |
| `export WSO2_APIM_ADMIN_PASSWORD=...`, then name the variable | `--password ...` |

A module sees only the `WSO2_<NAMESPACE>_*` variables of its own namespace,
so a secret a command needs reaches it by variable name. A token a command
returns on purpose, such as `apim api get-token` or `apim gateway register`,
is shown once, with a note on standard error, and is written nowhere.

### 7. Destructive commands confirm on the command line

A module cannot prompt. Without `--yes` a delete says what it would delete
and calls nothing; with it, the deletion runs.

```text
ws apim api delete <api>          # reports, exits 0, deletes nothing
ws apim api delete <api> --yes    # deletes
```

### 8. Return data, not text

A command returns a typed result and the shell renders it as a table or as
JSON, so `--output json` works for every command without the module doing
anything. A module never writes to standard output: that stream carries
protocol frames.

```text
ws apim api list -o json | jq '.[] | select(.status == "DEPLOYED") | .id'
```

### 9. Errors are typed, with the recovery in the message

| Do | Don't |
| --- | --- |
| `apim.not_deployed`: "API x is not deployed. Run ws apim api deploy <api> --gateway-id <gateway>." | `Error: 404` |
| `apim.not_authorized` on a 403, so an administrator fixes permissions | A retry hint on a permission failure |

Codes are `<namespace>.<snake_case>`, stay stable across releases, and the
shell maps the problem's category to the exit class (75 for a failure the
product service reported). See the
[exit classes](../reference/commands.md#exit-classes).

### 10. Help works offline, and every description starts with a verb

| Do | Don't |
| --- | --- |
| "Lists the APIs of one project." | "API listing command" |
| "Deploys an API to a registered gateway." | "Deploy" |

Help comes from the command tree recorded at installation, so it works with
no login and no network call. The details are under
[help behavior](#help-behavior) below.

## Help behavior

- `ws help` and `ws --help` show shell commands and known product namespaces.
  The root page distinguishes installed products from those available to
  install. It works offline and does not require a context or login.
- `ws help <namespace>` and `ws <namespace> --help` show the installed
  product's command groups and actions. `ws help <namespace> <command>` and
  `ws <namespace> <command> --help` show help for that command. Product help
  comes from the command tree recorded at installation and must work without
  launching the module or contacting the product.
- A group named without an action should show its available actions and exit
  successfully. An unknown command or flag should fail with a usage error,
  name the bad input, and point to the nearest relevant help page. It must not
  silently show a parent page as if the input were valid.
- Give every visible command a short sentence that starts with a verb and
  describes the result. Describe required arguments, accepted flags, and any
  meaningful defaults in the command's documentation. Help must list only
  commands and flags the current installation accepts. Mark a command's
  provider limits when a namespace covers more than one provider.
- Keep examples beside the command in its module documentation. The current
  product help renderer displays a short description, usage, child commands,
  and flags; it does not display Cobra `Long` or `Example` text. Do not rely on
  those fields to teach an operation until the help renderer supports them.

## Review a new command

Before merging, check these points against both the command tree and its docs:

1. The owner is correct: shell for shared policy, product namespace for product
   operations.
2. The path follows namespace, singular resource, action. Its nouns, verbs,
   arguments, and flags follow the rules above, and no operation has two
   verbs.
3. Flags reuse existing names, and no secret takes a value on the command
   line.
4. Root, group, and leaf help lead users to the command without login or a
   network call. Unknown input fails with a useful suggestion.
5. Each documented example can be run with the flags and arguments the command
   declares. The command appears in the module's declared tree, which the
   shell uses to parse and render help.
6. Every refusal carries a code and a recovery, and a destructive action
   needs `--yes`.

See the [command reference](../reference/commands.md) for existing shell
commands and the [module guide](build-module-quickstart.md) for declaring a
product command tree.
