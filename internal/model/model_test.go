package model

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseAmount(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int64
		err  error
	}{
		{"10", 1000, nil},
		{"10.5", 1050, nil},
		{"0.01", 1, nil},
		{".5", 50, nil},
		{json.Number("85.50"), 8550, nil},
		{0.1, 10, nil},
		{"9999999.99", 999999999, nil},
		{nil, 0, ErrAmountMissing},
		{"", 0, ErrAmountMissing},
		{"0", 0, ErrAmountTooLow},
		{"0.00", 0, ErrAmountTooLow},
		{"1.005", 0, ErrAmountDecimals},
		{"-1", 0, ErrAmountInvalid},
		{"1e3", 0, ErrAmountInvalid},
		{"1,000", 0, ErrAmountInvalid},
		{"10.", 0, ErrAmountInvalid},
		{true, 0, ErrAmountInvalid},
		{"99999999999999999999", 0, ErrAmountInvalid},
	} {
		got, err := ParseAmount(tc.in)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("ParseAmount(%v) = %d, %v; want %d, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	for in, want := range map[int64]string{0: "0.00", 5: "0.05", 1050: "10.50", -250: "-2.50"} {
		if got := FormatAmount(in); got != want {
			t.Errorf("FormatAmount(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestScenarioFor(t *testing.T) {
	for number, want := range map[string]Scenario{
		"0241234567":       ScenarioSuccess,
		"0240000001":       ScenarioDeclined,
		"+233 24 000 0002": ScenarioInsufficientFunds,
		"4000000000000004": ScenarioNeverSettles,
		"":                 ScenarioSuccess,
	} {
		if got := ScenarioFor(number); got.Key != want.Key {
			t.Errorf("ScenarioFor(%q) = %s, want %s", number, got.Key, want.Key)
		}
	}
}
