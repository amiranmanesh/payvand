# Changelog

All notable changes to Payvand are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **PayPing moved from API v2 to API v3.** PayPing has published v3 as its
  payment API; v2 was what Payvand spoke. The endpoints, the identifiers and
  the callback shape all changed, so this is a breaking change for the PayPing
  gateway alone:
  - `Purchase` posts to `/v3/pay` and `PurchaseResponse.Token` is now the
    **payment code**, not the v2 reference. The payer is sent to the address
    PayPing returns, falling back to `/v3/pay/start/{paymentCode}`.
  - `Verify` posts to `/v3/pay/verify` and needs **both** the payment code and
    the numeric `paymentRefId` — v2 keyed on the reference alone. Build the
    request with `Callback.VerifyRequest`, which carries both, and keep passing
    the amount from your own order record.
  - The callback is an `x-www-form-urlencoded` **POST** carrying `status`,
    `errorCode` and a `data` JSON document. `ParseCallback` flattens the
    document into `Callback.Values`, so raw field lookups keep working, and
    still reads the older flat query string.
- The amount unit is unchanged: PayPing's specification states that every one
  of its services works in **Toman**, which is what Payvand has been sending
  and converting to. The unit of every other gateway was re-checked against the
  provider's own documentation in the same pass and none of them moved.

### Fixed

- **YekPay purchases went to the SOAP endpoint.** `Purchase` posted its JSON
  body to `/api/payment/server`, which is YekPay's SOAP service and answers a
  JSON body with a SOAP fault. It posts to the REST endpoint
  `/api/payment/request` now, which is what the provider documents for a JSON
  client.

### Added

- **YekPay sandbox.** `WithSandbox(true)` switches to `api.ypsapi.com` and its
  `/api/sandbox/*` paths — the sandbox is not a mirror of the production paths,
  so the host and the path set are chosen together. The merchant id stays
  yours, and the test page lets the tester pick success or failure.
- `ErrVerificationPending`: the provider accepted the verification but has not
  settled it yet. PayPing answers `202`/`502` while it is still working; the
  documented recovery is to call `Verify` again, never to charge the payer a
  second time.
- PayPing gained `Refund` (`/v3/pay/reverse`, whole payment back to the payer's
  card within thirty minutes of verification), `Inquiry` (the payment report
  PayPing documents for a callback that never arrived) and split settlement.
- PayPing options: `WithReversible` — required at creation time before a
  reversal is accepted — `WithBlockedSettlement`, and `WithMultiplexing`, whose
  shares are checked to add up to the requested amount before anything is sent.

## [1.2.0] — 2026-07-31

A security and money-safety pass over every gateway. Two changes alter what
`Verify` returns for flows that used to look successful; both are listed under
Changed, and both were returning success for something that was not one.

**Upgrade before the next settlement run.** The signatures are unchanged, so
`go get -u` is the whole upgrade, but read Changed first: code that only tests
`err != nil` will start seeing errors for repeated callbacks. That is the
point — the alternative was fulfilling those orders twice.

This is a minor release rather than a major one on purpose. The behaviour that
changed was unsafe, and a `/v2` import path would leave everyone who does not
notice it on the version with the holes in it.

### Security

- **AsanPardakht settled an unpaid order.** `Verify` let
  `VerifyRequest.Extra["pay_gate_tran_id"]` replace the transaction id it had
  just looked up, and `Callback.VerifyRequest` copies the whole callback map
  into `Extra` — so returning to the callback URL with any transaction id
  marked the order paid. The provider lookup is now the only source of the
  settlement key; a supplied value is cross-checked and reported when it
  disagrees.
- **Sepehr settled any receipt against any order.** A "Duplicate" advice —
  the receipt was settled by an earlier call — was read as success, so a
  receipt from a cheap payment settled an expensive order. It is
  `ErrAlreadyVerified` now, and verification also checks the invoice and amount
  the callback claimed against the caller's own record.
- **Terminal credentials reached the logs.** Iranian gateways authenticate on
  the request body, and the whole body went into the log fields — including on
  the error path, which fires on any network failure. Credential fields are
  masked in both directions now. `Raw` and `Extra` are deliberately untouched;
  `SECURITY.md` says which is which.
- **Callback values went into URLs unescaped.** Jibit's purchase id and
  Digipay's tracking code both arrive from the payer's browser and were
  concatenated into request URLs, where a `?` or a `../` re-targets the call.
  These, and Vandar's refund path, are escaped.
- **Refunds were replayed by the retry policy.** `WithRetry` applied to every
  call, so a reversal that lost its answer to a timeout was sent again. Refunds
  and reversals now attempt exactly once.

### Fixed

- Sixteen gateways adopted the amount the provider reported instead of checking
  it against the amount that was ordered, so a token replayed from a cheaper
  payment verified cleanly. They reconcile through the new `SettledAmount`, and
  SnappPay does it before settling rather than after.
- NextPay reported every successful refund as a failure: the refund answers
  with -90, not with the verify success code.
- Zibal reported a paid-but-unverified transaction as verified, so a merchant
  recovering a lost callback never made the call that keeps the money.
- Mellat aborted a retried verification before the settle call, which is the
  call that keeps the money; its inquiry reported live transactions as failed.
- Vandar's settled amount arrives as a decimal string and was silently dropped
  by an integer parse.
- PayWeb decided a verification from the HTTP status alone.
- YekPay never reported the settled amount at all.

### Added

- `payvand.SettledAmount` — the settled-versus-ordered comparison every
  `Verify` now performs, exported for the same check against an inquiry.
- `Money.Equal`, which compares worth across Rial and Toman.
- `torobpay.WithSettle` — TorobPay serves the same endpoint paths as SnappPay,
  where an unsettled payment is reverted, and its documentation is not public.
  The call is available and off by default; ask the provider which your
  contract is.

### Changed

- Gateways whose provider signals a repeated verification — Zarinpal, Zibal,
  IDPay, BitPay, NextPay, Sepehr, Mellat — return an error wrapping
  `ErrAlreadyVerified` instead of a fresh success. Treat it as "already paid";
  the point is that the order is not fulfilled twice.
- `Verify` returns `ErrAmountMismatch` where it used to return the provider's
  amount and no error.

## [1.1.0] — 2026-07-31

Five new providers, four of which lend rather than move money from a card. The
public API is unchanged: the buy-now-pay-later gateways implement the same
`Gateway` interface, so an existing call site reaches them by name alone.

### Added

**Gateways (5)**

- `jibit` — Jibit's proxy payment gateway (PPG v3): purchase, verify, inquiry,
  partial refunds and `Reverse` for a payment that was taken but not verified.
- `snapppay` — SnappPay's online instalments, with `Eligible` for the
  pre-checkout credit decision, verification and settlement in one `Verify`,
  `Settle`, `Cancel` and `Update`.
- `torobpay` — TorobPay's four-instalment online credit, with `Cancel` and the
  status inquiry.
- `digipay` — Digipay's wallet, credit, BNPL, credit-card and IPG products,
  selected with the ticket type, plus `Reverse`, `Deliver` and `RefundStatus`.
- `tara` — Tara's club credit, with the itemised invoice it settles against and
  the POST form it redirects with.

**Provider features**

- Basket and invoice builders, so the BNPL providers see the real order lines:
  `snapppay.WithCartBuilder`, `torobpay.WithCartBuilder`,
  `digipay.WithBasketBuilder`, `tara.WithInvoiceBuilder`. Each has a default
  that sends a single line covering the whole order.
- SnappPay's settlement runs inside `Verify` and can be deferred with
  `snapppay.WithAutoSettle(false)`.
- Digipay verifies against the ticket type reported by the callback, so a payer
  who switched product inside the Digipay app is still verified correctly.

**Internal**

- `internal/tokenauth`, a bearer token cache that authenticates lazily, renews
  a rejected token and replays the call exactly once.

### Notes

- Still no dependencies: the OAuth password grants, the multipart form Digipay
  wants and the token cache are all standard library.

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

[Unreleased]: https://github.com/amiranmanesh/payvand/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/amiranmanesh/payvand/releases/tag/v1.2.0
[1.1.0]: https://github.com/amiranmanesh/payvand/releases/tag/v1.1.0
[1.0.0]: https://github.com/amiranmanesh/payvand/releases/tag/v1.0.0
