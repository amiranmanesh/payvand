# Contributing to Payvand

Thanks for helping. This document is short because the rules are few.

## The one hard rule

**Payvand depends on the Go standard library and nothing else.** No HTTP
client, no SOAP library, no logging framework, no assertion helper. `make
deps-check` and the CI pipeline fail the build if a `require` block appears in
`go.mod`.

If a feature seems to need a dependency, it almost certainly belongs in
`internal/` as a small, focused helper — that is how the SOAP client, the
crypto primitives and the retrying HTTP client came to be.

## Getting set up

```bash
git clone https://github.com/amiranmanesh/payvand
cd payvand
make test        # go test -race ./...
make lint        # gofmt check + go vet
make ci          # everything the pipeline runs
```

Go 1.26 or newer.

## Adding a gateway

A new provider is expected to bring four things.

**1. The package**, `gateway/<name>/`, with exactly three files, matching the
shape of the existing ones:

| File | Contents |
|---|---|
| `<name>.go` | the `core.Gateway` implementation, endpoints as constants, response-code mapping |
| `dto.go` | the wire structs, unexported |
| `options.go` | the provider's opt-in options, each returning a `core.Option` |

Checklist for the implementation:

- Register the factory from `init`, and expose the name as
  `const Name core.Name = "<name>"`.
- Embed `core.Unsupported` and override only the operations the provider truly
  offers. Report the truth in `Capabilities`.
- Validate credentials in `New` and return `core.ErrInvalidConfig`, so a
  misconfigured terminal fails at wiring time rather than at payment time.
- Honour `core.Options.BaseURL`; every endpoint must be reachable through it.
  This is what makes the package testable.
- Convert amounts with `Money.Rial()` or `Money.Toman()` — never assume a unit.
- Wrap failures in `core.NewError(Name, op, sentinel).WithCode(...).WithMessage(...)`.
- If the provider publishes a code table, expose it as `func Message(code) string`
  in English.

**2. Tests**, `gateway/<name>/<name>_test.go`, in package `<name>_test`, driven
by `internal/testutil`. Purchase, verify and callback parsing at minimum, plus
one failure path. No network, no credentials, no build tags.

**3. Documentation**: a row in the gateway table and the credentials table of
the `README.md`, an entry in the options table, and a package doc comment that
states which `core.Config` field carries which credential.

**4. A changelog entry** under `## [Unreleased]`.

## Registering a private gateway instead

If the provider is in-house or under NDA, you do not need to upstream it.
Implement `core.Gateway` in your own module and call `payvand.Register` —
`payvand.New("your-gateway", cfg)` will then work like any built-in one. See
the *Extending* page of the wiki.

## Style

- Comment every exported symbol, and every struct field. The existing files are
  the reference.
- Write comments that explain **why** — the protocol quirk, the provider rule —
  not what the next line does.
- Keep provider quirks documented at the place they are handled. "TOP expects
  the mobile number as a number, so the leading zero is dropped" is worth a
  line; the code alone is not.
- `gofmt` decides formatting arguments.

## Commits and pull requests

- Conventional commits: `feat(zibal): …`, `fix(mellat): …`, `docs: …`,
  `test(core): …`, `ci: …`, `chore: …`.
- One logical change per commit; a new gateway is one commit, its tests
  included.
- Every commit should build and pass `make ci` on its own.
- Describe, in the pull request, how you verified the change against the real
  provider — sandbox terminal, staging account, or "documentation only".

## Reporting a provider change

Iranian PSPs change endpoints and response codes without notice. When you hit
one, open an issue with the raw request and response (**with the credentials,
card numbers and tokens redacted**) and the date. That is usually enough to fix
it in one commit.

## Security

Do not open a public issue for a vulnerability; see [SECURITY.md](SECURITY.md).
