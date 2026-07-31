package core

import "strconv"

// Currency identifies the unit an [Money.Amount] is expressed in.
//
// Iranian gateways are inconsistent: most of them expect Rial (IRR) while a
// few (PayPing, some Zarinpal setups) expect Toman (IRT). Payvand always keeps
// the caller's unit explicit and lets every gateway convert to whatever its own
// API expects, so the caller never has to multiply or divide by ten.
type Currency uint8

const (
	// IRR is the Iranian Rial. It is the zero value, therefore an
	// uninitialised Money is interpreted as Rial.
	IRR Currency = iota
	// IRT is the Iranian Toman, equal to ten Rial.
	IRT
)

// String returns the ISO-like code of the currency ("IRR" or "IRT").
func (c Currency) String() string {
	switch c {
	case IRT:
		return "IRT"
	default:
		return "IRR"
	}
}

// Money is an amount bound to the unit it was expressed in.
//
// Build it with [Rial] or [Toman] instead of using the struct literal, so the
// unit is always stated at the call site:
//
//	req.Amount = payvand.Toman(15_000)
type Money struct {
	// Amount is the numeric value expressed in Currency.
	Amount int64
	// Currency is the unit of Amount. The zero value is [IRR].
	Currency Currency
}

// Rial builds a [Money] expressed in Iranian Rial.
func Rial(amount int64) Money { return Money{Amount: amount, Currency: IRR} }

// Toman builds a [Money] expressed in Iranian Toman.
func Toman(amount int64) Money { return Money{Amount: amount, Currency: IRT} }

// Rial returns the value converted to Iranian Rial.
func (m Money) Rial() int64 {
	if m.Currency == IRT {
		return m.Amount * 10
	}
	return m.Amount
}

// Toman returns the value converted to Iranian Toman. Rial values that are not
// a multiple of ten are truncated, which mirrors what the gateways themselves
// do with sub-Toman remainders.
func (m Money) Toman() int64 {
	if m.Currency == IRT {
		return m.Amount
	}
	return m.Amount / 10
}

// In converts the value to the given currency and returns a new [Money].
func (m Money) In(c Currency) Money {
	switch c {
	case IRT:
		return Money{Amount: m.Toman(), Currency: IRT}
	default:
		return Money{Amount: m.Rial(), Currency: IRR}
	}
}

// IsZero reports whether the amount is zero (in any unit).
func (m Money) IsZero() bool { return m.Amount == 0 }

// Equal reports whether two amounts are worth the same, whatever unit each of
// them is expressed in.
func (m Money) Equal(other Money) bool { return m.Rial() == other.Rial() }

// SettledAmount reconciles the amount a provider reports for a payment with the
// amount the caller asked to verify, and is the amount every [Gateway.Verify]
// must report.
//
// Verifying is the moment the merchant learns what it was actually paid, and
// the request amount comes from the merchant's own order while the reported one
// comes from the provider. When they disagree, the payment settled for
// something other than what was ordered — an error wrapping [ErrAmountMismatch]
// is the only safe answer, because the alternative is a caller shipping goods
// against a cheaper payment replayed onto the order.
//
// A non-positive amount on either side means "not stated": providers that echo
// no amount leave nothing to compare, and callers that pass none are asking to
// be told rather than checked. Gateways whose provider always states an amount
// should reject a missing one themselves rather than rely on this.
func SettledAmount(gateway Name, requested, reported Money) (Money, error) {
	switch {
	case reported.Amount <= 0:
		return requested, nil
	case requested.Amount <= 0:
		return reported, nil
	case !reported.Equal(requested):
		return Money{}, NewError(gateway, "verify", ErrAmountMismatch).
			WithMessage("the provider settled " + reported.String() + ", not " + requested.String())
	default:
		return reported, nil
	}
}

// String renders the amount and its unit, e.g. "15000 IRT".
func (m Money) String() string {
	return strconv.FormatInt(m.Amount, 10) + " " + m.Currency.String()
}
