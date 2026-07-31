package core_test

import (
	"errors"
	"testing"

	"github.com/amiranmanesh/payvand/core"
)

func TestMoneyConversion(t *testing.T) {
	cases := []struct {
		name      string
		money     core.Money
		wantRial  int64
		wantToman int64
		wantLabel string
	}{
		{name: "rial stays rial", money: core.Rial(150_000), wantRial: 150_000, wantToman: 15_000, wantLabel: "150000 IRR"},
		{name: "toman becomes rial", money: core.Toman(15_000), wantRial: 150_000, wantToman: 15_000, wantLabel: "15000 IRT"},
		{name: "zero value is rial", money: core.Money{Amount: 500}, wantRial: 500, wantToman: 50, wantLabel: "500 IRR"},
		{name: "rial remainder truncates", money: core.Rial(105), wantRial: 105, wantToman: 10, wantLabel: "105 IRR"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.money.Rial(); got != tc.wantRial {
				t.Errorf("Rial() = %d, want %d", got, tc.wantRial)
			}
			if got := tc.money.Toman(); got != tc.wantToman {
				t.Errorf("Toman() = %d, want %d", got, tc.wantToman)
			}
			if got := tc.money.String(); got != tc.wantLabel {
				t.Errorf("String() = %q, want %q", got, tc.wantLabel)
			}
		})
	}
}

func TestMoneyIn(t *testing.T) {
	converted := core.Toman(2_500).In(core.IRR)
	if converted.Amount != 25_000 || converted.Currency != core.IRR {
		t.Fatalf("In(IRR) = %v, want 25000 IRR", converted)
	}
	back := converted.In(core.IRT)
	if back.Amount != 2_500 || back.Currency != core.IRT {
		t.Fatalf("In(IRT) = %v, want 2500 IRT", back)
	}
}

func TestSettledAmount(t *testing.T) {
	cases := []struct {
		name      string
		requested core.Money
		reported  core.Money
		want      core.Money
		wantErr   bool
	}{
		{name: "equal amounts", requested: core.Rial(150_000), reported: core.Rial(150_000), want: core.Rial(150_000)},
		{name: "equal across units", requested: core.Toman(15_000), reported: core.Rial(150_000), want: core.Rial(150_000)},
		{name: "provider settled less", requested: core.Rial(150_000), reported: core.Rial(15_000), wantErr: true},
		{name: "provider settled more", requested: core.Rial(150_000), reported: core.Rial(1_500_000), wantErr: true},
		{name: "provider states nothing", requested: core.Rial(150_000), reported: core.Money{}, want: core.Rial(150_000)},
		{name: "caller states nothing", requested: core.Money{}, reported: core.Rial(150_000), want: core.Rial(150_000)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := core.SettledAmount("gw", tc.requested, tc.reported)
			if tc.wantErr {
				if !errors.Is(err, core.ErrAmountMismatch) {
					t.Fatalf("error = %v, want ErrAmountMismatch", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("SettledAmount() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("SettledAmount() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMoneyIsZero(t *testing.T) {
	if !(core.Money{}).IsZero() {
		t.Error("the zero value must report IsZero")
	}
	if core.Rial(1).IsZero() {
		t.Error("a non zero amount must not report IsZero")
	}
}
