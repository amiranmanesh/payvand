# Changelog

All notable changes to Payvand are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

## [1.0.0] — 2026-07-31

First stable release. The public API of `payvand` and `payvand/core` is frozen
for the whole 1.x line.

### Added

**Core**

- `Gateway`, the single interface every provider implements: `Purchase`,
  `Verify`, `Refund`, `Inquiry`, `ParseCallback`, plus `Name` and
  `Capabilities`.
- Provider independent request and response types, with `Money` carrying the
  caller's unit so each gateway converts to the Rial or Toman its own API
  expects.
- `Redirect`, covering both the plain GET redirect and the auto-submitting
  form the POST gateways need.
- `Callback` and `CallbackValues`, normalising the query-string and POST-form
  differences between the providers.
- Registry (`Register`, `New`, `Registered`, `IsRegistered`) so the gateway
  name can come from configuration or a database row.
- `Client` (`payvand.Init`) holding the options shared by an application.
- Sentinel errors and the rich `Error` type carrying the provider's own code
  and message.

**Gateways (20)**

- PSPs: Zarinpal, Zibal, Vandar, PayWeb, IDPay, Pay.ir, NextPay, PayPing,
  BitPay.ir, YekPay.
- Bank acquirers: Sadad (Bank Melli), Parsian, Iran Kish, Mellat
  (Behpardakht), Saman (SEP), Pasargad, AsanPardakht, Sepehr (Bank Saderat).
- In-app: TOP (Taban Ati Pardaz).
- `virtual`, an in-memory gateway that runs the whole cycle offline.

**Provider features**

- Multi-step settlement hidden inside `Verify`: Mellat (verify + settle),
  AsanPardakht (result + verify + settlement), Vandar (transaction + verify),
  Pasargad (check + verify).
- Split settlement for Zarinpal (wages), Zibal (amount and percentage),
  Parsian (IBAN) and AsanPardakht.
- Signed protocols on the standard library: 3DES-ECB (Sadad), AES-CBC inside
  an RSA envelope (Iran Kish), RSA-signed bodies (Pasargad, accepting both PEM
  and .NET `RSAKeyValue` keys).
- SOAP 1.1 client on `encoding/xml` for Parsian and Mellat.
- Functional options per gateway; every provider extra is opt-in.

**Transport**

- Timeout, retry on network errors and 5xx, custom headers, pluggable HTTP
  client and a logger interface with a `log/slog` adapter.

**Project**

- Tests for every gateway plus core, transport and crypto, all driven by
  `httptest` — no network and no credentials.
- Runnable examples: `basic`, `multigateway`, `webshop`.
- Makefile, CI workflow and a `deps-check` target that fails the build if a
  third party dependency ever appears in `go.mod`.

### Notes

- Payvand imports nothing outside the Go standard library, and it is meant to
  stay that way.
- Go 1.26 or newer is required.

[Unreleased]: https://github.com/amiranmanesh/payvand/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/amiranmanesh/payvand/releases/tag/v1.0.0
