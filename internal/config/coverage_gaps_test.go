package config

// Targeted tests for helpers with thin coverage (project-wide raise, 1.8.27).

import (
	"testing"
	"time"
)

func TestAsDuration(t *testing.T) {
	cases := []struct {
		in   interface{}
		want time.Duration
		ok   bool
	}{
		{"5s", 5 * time.Second, true},
		{"1m", time.Minute, true},
		{"not-a-duration", 0, false},
		{int(30), 30 * time.Second, true},
		{int64(7), 7 * time.Second, true},
		{float64(1.5), 1500 * time.Millisecond, true},
		{true, 0, false}, // unsupported type
	}
	for _, c := range cases {
		got, ok := asDuration(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("asDuration(%v) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeJSONLanguages(t *testing.T) {
	// Expanded object form → codes extracted, config rewritten.
	in := []byte(`{"languages":[{"code":"pl","name":"Polski"},{"code":"en","name":"English"}],"domain":"x"}`)
	out, expanded, err := normalizeJSONLanguages(in)
	if err != nil || len(expanded) != 2 || expanded[0].Code != "pl" {
		t.Fatalf("expanded form: %v %v", expanded, err)
	}
	if string(out) == string(in) {
		t.Fatal("expanded form should rewrite languages to plain codes")
	}

	// Plain string list stays untouched (not the expanded form).
	plain := []byte(`{"languages":["pl","en"]}`)
	out, expanded, err = normalizeJSONLanguages(plain)
	if err != nil || expanded != nil || string(out) != string(plain) {
		t.Fatalf("plain list should pass through: %s %v %v", out, expanded, err)
	}

	// No languages key → passthrough.
	none := []byte(`{"domain":"x"}`)
	if out, expanded, err = normalizeJSONLanguages(none); err != nil || expanded != nil || string(out) != string(none) {
		t.Fatalf("no key should pass through: %s %v %v", out, expanded, err)
	}

	// Invalid JSON → error, data returned as-is.
	if _, _, err = normalizeJSONLanguages([]byte(`{bad`)); err == nil {
		t.Fatal("invalid JSON must error")
	}
}
