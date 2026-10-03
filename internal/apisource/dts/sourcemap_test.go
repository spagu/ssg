package dts

import (
	"reflect"
	"testing"
)

// TestDecodeVLQ checks single and multi-digit numbers, signs and errors.
func TestDecodeVLQ(t *testing.T) {
	cases := map[string][]int{
		"AAAA": {0, 0, 0, 0},
		"AAEA": {0, 0, 2, 0},
		"D":    {-1},
		"F":    {-2},
		"gB":   {16},
		"hB":   {-16},
		"2H":   {123},
	}
	for seg, want := range cases {
		got, err := decodeVLQ(seg)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("decodeVLQ(%q) = %v, %v; want %v", seg, got, err, want)
		}
	}
	for _, bad := range []string{"A!", "g", "gggggggggA"} {
		if _, err := decodeVLQ(bad); err == nil {
			t.Errorf("decodeVLQ(%q) did not fail", bad)
		}
	}
}

// TestParseSourceMap checks line mapping, deltas across lines, unmapped
// lines and sources that cannot be linked.
func TestParseSourceMap(t *testing.T) {
	data := `{"version":3,"sourceRoot":"../","sources":["src/a.ts","https://x/b.ts"],"mappings":"AAAA,EAAE;;AAEA;ACAA;A"}`
	m, err := parseSourceMap([]byte(data), "dist/a.d.ts.map")
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		line int
		file string
		orig int
		ok   bool
	}{
		{1, "src/a.ts", 1, true},
		{2, "", 0, false},
		{3, "src/a.ts", 3, true},
		{4, "", 0, false}, // second source is a URL
		{5, "", 0, false}, // a one-field segment has no source
		{6, "", 0, false}, // past the end
		{0, "", 0, false},
	}
	for _, c := range checks {
		f, l, ok := m.lookup(c.line)
		if f != c.file || l != c.orig || ok != c.ok {
			t.Errorf("lookup(%d) = %q %d %v, want %q %d %v", c.line, f, l, ok, c.file, c.orig, c.ok)
		}
	}
	var none *sourceMap
	if _, _, ok := none.lookup(1); ok {
		t.Error("nil map mapped a line")
	}
}

// TestParseSourceMapErrors checks malformed maps are refused.
func TestParseSourceMapErrors(t *testing.T) {
	for _, data := range []string{`{`, `{"version":2}`, `{"version":3,"sources":["a.ts"],"mappings":"A!"}`} {
		if _, err := parseSourceMap([]byte(data), "a.d.ts.map"); err == nil {
			t.Errorf("parseSourceMap(%s) did not fail", data)
		}
	}
}

// TestSourcePath checks paths leaving the root are dropped.
func TestSourcePath(t *testing.T) {
	cases := [][4]string{
		{"dist", "", "../src/a.ts", "src/a.ts"},
		{".", "", "../a.ts", ""},
		{".", "", "/abs/a.ts", ""},
		{".", "webpack://", "a.ts", ""},
	}
	for _, c := range cases {
		if got := sourcePath(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("sourcePath(%q, %q, %q) = %q, want %q", c[0], c[1], c[2], got, c[3])
		}
	}
}
