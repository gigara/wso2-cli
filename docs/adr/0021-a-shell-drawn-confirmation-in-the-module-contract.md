# ADR 0021: A Shell-Drawn Confirmation in the Module Contract

**Status:** Accepted

A product module asks the shell to confirm a destructive action with a
`Confirm` message carrying the question, and the shell answers `Confirmed`
or `ConfirmationRefused`. The shell decides whether anything may be asked,
draws the question through the same `internal/wizard` prompter as its own
`ws product remove`, and writes every refusal. The module decides what the
question says, because it alone knows what it is about to delete. This is
protocol version 3.

Before this, a module's delete command could only refuse without `--yes`
(`apim.invalid_argument`, exit class 64), because the contract gave it no
way to ask: a module may not write to standard output (ADR 0002) and never
draws a prompt (ADR 0003). The command guideline wants the standard model
instead, the one the shell already follows for its own destructive
commands: ask at a terminal, `--yes` answers without asking, and a
non-interactive run without `--yes` fails naming `--yes`.

Three properties hold.

**Whether to ask stays with the shell.** `mayPrompt` in `internal/app`
decides from `--no-input`, `WSO2_NO_INPUT`, and whether standard input is a
terminal, exactly as for the shell's own prompts (ADR 0017). A module cannot
observe the terminal and does not try: it sends the question and acts only
on `Confirmed`. Under a control that forbids prompting the shell answers
`ConfirmationRefused` carrying the same `shell.non_interactive` (usage, exit
class 64) that `ws product remove --no-input` returns, worded by the same
helper: the invoked command as the subject, `--yes` beside the one control
that fired. A no at the prompt, or Ctrl+C, is `shell.cancelled`. The module
returns the refusal unchanged, and the shell reports its own wording for a
refusal it issued, as it does for an access denial.

**`--yes` is the module's flag.** The shell never reads it. A module whose
command was given `--yes` sends no `Confirm` at all, so the request is
exactly as rare as the prompt: a scripted run never crosses the boundary for
it. The flag's name is rule 7 of the command guideline, which is what lets
the shell's refusal name it without being told.

**It is a new protocol version, not a capability.** A shell that does not
know `Confirm` fails closed on it, by the contract's own rule that an
unknown message kind is refused rather than skipped, and a module that asks
and is refused with a process failure has nothing to act on. The protocol
version is where that incompatibility is meant to be stated: a module built
against this SDK declares `protocolVersions: [3]`, the shell's window
becomes `[3, 2]`, and a shell still speaking `[2, 1]` refuses to install
the module (`modules.incompatible_protocol`) instead of launching it and
failing on its first delete. The release gate keeps the order: the shell
that speaks 3 ships before any module that requires it.

## Considered Options

- **A `required_capabilities` entry (`"confirm"`).** It also fails closed,
  at the handshake, and would let a module that never confirms keep
  declaring version 2. Rejected because the contract has defined no
  capability so far on purpose, and a version is the one compatibility
  signal the catalog, the installer, the release gate and `ws version`
  already read; a capability would be a second one for the same question.
- **Keeping `--yes` mandatory.** Honest and simple, and what the modules did.
  Rejected because it is not the model the shell's own commands follow, and
  because a person at a terminal is the common case.
- **Letting the module read `WSO2_NO_INPUT` and write the refusal itself.**
  The module already receives that variable. Rejected because the terminal
  check is the shell's, so the module could only get the non-interactive
  half right, and because the refusal's wording and exit class are shell
  policy, like an access denial's.
- **Carrying the answer as a bool rather than a refusal.** A no would then
  leave each module to invent its own outcome for it. Rejected so that
  declining reads the same under every product: `shell.cancelled`, nothing
  changed.

## Consequences

The contract grows by three message kinds, and the shell's `Session`
answers `Confirm` the way it answers `AcquireAccess`: one correlated
exchange at a time, bounded in number, with the module's deadline held
while a person is being waited for. `sdk/module` adds `ConfirmRequest`, the
`Confirmer` interface, and `Request.Prompt`; `sdk/testkit` scripts the
answer through `Invocation.Confirm` and records each question in
`Outcome.Confirmations`. The protocol version moves to 3, so the SDK
release that carries it is a new generation: every module built against it
needs a shell from this release or later, and the example, apim and iam
descriptors move with it. `ws apim api delete`, `ws apim project delete`
and `ws iam resource-server delete` ask instead of refusing, and keep
`--yes`.
