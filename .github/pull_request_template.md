# What this changes

<!-- One paragraph. If it fixes an issue, write "Fixes #123". -->

# How it was verified

<!-- Tick what applies. -->

- [ ] `make ci` passes
- [ ] tested against the provider's sandbox
- [ ] tested against a production terminal
- [ ] documentation only

# Checklist

- [ ] No new dependency — `go.mod` still has no `require` block
- [ ] Exported symbols and struct fields are commented
- [ ] Tests added or updated, driven by `internal/testutil`
- [ ] `Capabilities()` reflects what the provider actually supports
- [ ] Every endpoint honours `core.Options.BaseURL`
- [ ] README tables and `CHANGELOG.md` updated
- [ ] No credential, token or PAN in the diff
