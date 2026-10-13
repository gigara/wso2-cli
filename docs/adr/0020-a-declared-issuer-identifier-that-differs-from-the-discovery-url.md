# ADR 0020: A Declared Issuer Identifier That Differs from the Discovery URL

**Status:** Accepted

A context's login may declare `issuerId`, the issuer identifier its provider
names when that is not the URL the provider is served from. `login.issuer`
stays that URL: the shell reads the provider's OpenID configuration from it,
and sessions stay bound to it. When `issuerId` is declared, the shell expects
the configuration to name exactly `issuerId` as its issuer, and verifies every
identity token's `iss` against it. Without `issuerId`, nothing changes: the
provider must name `login.issuer`, as OpenID Connect Discovery requires.

Some providers do not follow that rule. A provider served from, say,
`https://idp.example.com` may name a bare identifier such as `example-idp` in
its discovery document and in every token it issues, because its server sets
`iss` from one deployment-wide setting. Once other relying parties trust that
identifier verbatim, changing it signs everyone out of every one of them and
needs all of them to change at once. Before this decision the shell refused
such a provider at discovery, and no context a person could write would reach
it.

The shell gives go-oidc the declared identifier through
`oidc.InsecureIssuerURLContext`. go-oidc then fetches the configuration from
`login.issuer` and uses `issuerId` as the provider's issuer, which is what its
identity token verifier checks `iss` against. That facility also turns off
go-oidc's own comparison of the configuration's issuer with the expected one,
so `internal/auth/oidcdiscovery` makes the comparison itself: the provider must
name exactly the declared `issuerId`. Every discovery the shell runs at the
login issuer goes through it: browser and device login, renewal, token
exchange, client credentials, revocation, end-session, and `wso2 doctor`.

The change is limited to the issuer identifier:

- `issuerId` is honoured only when a context declares it. The shell never
  accepts whatever identifier a provider names, and never guesses one.
- The configuration is still fetched from `login.issuer`, so `login.issuer`
  is still checked as an `https` URL (or `http` on loopback).
- Every endpoint in the configuration must still be `https` (or loopback
  `http`). Only its `issuer` member is compared with `issuerId` instead of
  being read as a URL.
- Token signatures are still verified against the provider's published keys,
  and the identity token's issuer is still checked, against `issuerId`.
- An `issuerId` that is an `http` or `https` URL follows the same
  `https`-or-loopback rule as `login.issuer`. A URN or an identifier with any
  other scheme is an identifier only, since nothing is sent to it. One that is
  empty, contains whitespace, or equals `login.issuer` (a trailing `/` aside)
  is refused.
- A product grant's own issuer cannot declare one, because no deployment
  needs it.

When a provider names an issuer the context does not expect, the refusal says
so and quotes the identifier the provider names, escaped and limited in
length, with the recovery "set `login.issuerId`". It keeps the code
`auth.discovery_failed`, because the list of stable codes does not change. The
context's own issuer is not repeated, for the same reason as every other
configured URL the shell refuses.

This narrows the exact-match rule of ADR 0005 only on the shell's side and
only for the issuer identifier. Resource servers that verify the tokens are
not affected: they already trust the identifier the provider names. Sessions
stay bound to `login.issuer` (ADR 0016), so declaring or changing `issuerId`
ends no session.

## Considered Options

- **Changing the deployment** so the provider names its URL. This is the
  standard-conforming fix, but it signs everyone out of every relying party
  that trusts the current identifier and needs all of them to change at once.
  The shell would also stay unable to reach any other provider set up the
  same way.
- **Making `login.issuer` the identifier and adding a separate discovery
  URL.** This also works, but it changes what `login.issuer` means in every
  existing context, and sessions, `whoami`, `doctor` and the session issuer
  check all key on `login.issuer`. Keeping `login.issuer` as the URL and
  adding an optional identifier changes nothing for existing contexts.
- **Accepting any issuer the provider names.** This needs no configuration,
  but then the issuer check proves nothing: a provider, or anything that
  can answer at that URL, could name any issuer.

## Consequences

- A context for such a provider declares `issuerId`. It can come from an
  input file through `wso2 context apply`, or from
  `wso2 context create --issuer <url> --issuer-id <id>`.
- The schema version stays 4: the member is optional and absent from every
  existing document.
- go-oidc's `InsecureIssuerURLContext` turns off a check, and the shell
  replaces it. A change to how go-oidc applies that context must be reviewed
  against this decision.
