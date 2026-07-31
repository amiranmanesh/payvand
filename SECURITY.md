# Security policy

## Supported versions

| Version | Supported |
|---|---|
| 1.x | ✅ |
| < 1.0 | ❌ |

## Reporting a vulnerability

Please do **not** open a public issue.

Use GitHub's private reporting —
[Security → Report a vulnerability](https://github.com/amiranmanesh/payvand/security/advisories/new)
— and include:

- the affected version and gateway,
- what an attacker can achieve,
- a reproduction, with **all credentials, tokens, card numbers and IBANs
  redacted**.

You can expect a first answer within a week and a fix or a plan within a
month, depending on severity.

## What counts as a vulnerability here

Payvand handles money and terminal credentials, so the interesting classes are:

- a verification path that reports success when the provider did not settle
  the payment,
- an amount that can be influenced by the payer (the callback is never
  trusted for amounts — a way around that is a bug),
- credentials, tokens or PANs leaking into logs, errors or the `Raw` fields,
- a flaw in the signing or envelope code under `internal/cryptox`,
- a callback parser that can be made to attribute a payment to the wrong
  order.

## What does not

- A provider rejecting a request because its own API changed. Open a normal
  issue.
- Weak algorithms mandated by an acquirer's protocol — 3DES-ECB for Sadad,
  RSA over SHA-1 for Pasargad. They are the providers' contracts; Payvand
  implements them and documents them as such.

## Guidance for integrators

- Never build the verification amount from the callback. Read it from your own
  order — `Callback.VerifyRequest(amount)` is shaped to make that the easy path.
- Treat `Callback.Succeeded` as a hint. A payment is final only when `Verify`
  returns without error.
- Store the payment token before redirecting the payer; it is the only handle
  on the transaction afterwards.
- Keep terminal keys out of the repository. `.gitignore` already excludes
  `*.pem`, `*.key`, `*.p12` and `keys/`.
- `Raw`, `PurchaseResponse.Extra` and the logger receive provider payloads.
  Redact them before they reach a log aggregator.
- Leave `WithSkipTLSVerify` off unless a Shaparak host genuinely serves an
  incomplete chain, and scope it to that gateway only.
