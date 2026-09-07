package generator

// A shortcode attribute is always a string, so without a conversion a numeric
// option could not be used for anything numeric (#253).

import (
	"strings"
	"testing"
)

// TestNumbersArriveAsText: what `[reviews limit="3"]` actually hands a template.
func TestNumbersArriveAsText(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{"3", 3},
		{" 7 ", 7}, // trimmed: an attribute may carry spacing
		{"-2", -2},
		{"4.9", 4}, // truncates toward zero, like Go's own int()
		{3, 3},
		{3.7, 3},
		{int64(9), 9},
	}
	for _, c := range cases {
		got, err := tmplInt(c.in)
		if err != nil {
			t.Errorf("int(%v): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("int(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestFloatFromText: the same conversion keeping the fraction, for `filter
// "rating" "ge" (float .Attrs.min)`.
func TestFloatFromText(t *testing.T) {
	got, err := tmplFloat("4.5")
	if err != nil {
		t.Fatalf("float: %v", err)
	}
	if got != 4.5 {
		t.Errorf("float(\"4.5\") = %v, want 4.5", got)
	}
}

// TestNotANumberIsAnError: `[reviews limit="six"]` is a mistake worth seeing.
// Silently becoming 0 would render an empty listing for no visible reason.
func TestNotANumberIsAnError(t *testing.T) {
	for _, bad := range []interface{}{"six", "", "  ", "3px", nil, []string{"3"}} {
		if _, err := tmplInt(bad); err == nil {
			t.Errorf("int(%v) should be an error", bad)
		}
		if _, err := tmplFloat(bad); err == nil {
			t.Errorf("float(%v) should be an error", bad)
		}
	}
}

// TestArithmeticAcceptsNumericText: the wider half of the fix — add/sub/mul/div
// take an attribute directly, so `add 0 .Attrs.limit` is no longer a puzzle.
func TestArithmeticAcceptsNumericText(t *testing.T) {
	got, err := tmplAdd(1, "2")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	// An integer spelling stays an integer: 3, not 3.0.
	if got != int64(3) {
		t.Errorf("add(1, \"2\") = %v (%T), want int64(3)", got, got)
	}
	if got, err := tmplMul("2.5", 2); err != nil || got != 5.0 {
		t.Errorf("mul(\"2.5\", 2) = %v, %v", got, err)
	}
	if _, err := tmplAdd(1, "two"); err == nil {
		t.Error("add with non-numeric text must still fail")
	} else if !strings.Contains(err.Error(), "must be numbers") {
		t.Errorf("unhelpful error: %v", err)
	}
}
