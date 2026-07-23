package generator

import (
	"strings"
	"testing"
)

// TPL-003: Go templates have no arithmetic, so a theme could not split a list
// into columns or compute "page N of M" without preprocessing in Go.
func TestArithmeticHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  func() (interface{}, error)
		want interface{}
	}{
		{"add ints", func() (interface{}, error) { return tmplAdd(2, 3) }, int64(5)},
		{"sub ints", func() (interface{}, error) { return tmplSub(5, 2) }, int64(3)},
		{"mul ints", func() (interface{}, error) { return tmplMul(3, 4) }, int64(12)},
		{"div ints truncates", func() (interface{}, error) { return tmplDiv(7, 2) }, int64(3)},
		{"div exact", func() (interface{}, error) { return tmplDiv(12, 4) }, int64(3)},
		{"float operand stays float", func() (interface{}, error) { return tmplAdd(1.5, 2) }, 3.5},
		{"float division", func() (interface{}, error) { return tmplDiv(7.0, 2.0) }, 3.5},
		{"float operands stay float", func() (interface{}, error) { return tmplAdd(2.0, 3.0) }, float64(5)},
		{"mixed kinds divide fractionally", func() (interface{}, error) { return tmplDiv(7.0, 2) }, 3.5},
		{"column split: ceil via (n+1)/2", func() (interface{}, error) { return tmplDiv(11+1, 2) }, int64(6)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.got()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("= %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestArithmeticHelperErrors(t *testing.T) {
	if _, err := tmplDiv(1, 0); err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("div by zero = %v, want a division-by-zero error", err)
	}
	if _, err := tmplAdd("two", 3); err == nil || !strings.Contains(err.Error(), "must be numbers") {
		t.Errorf("add with a string = %v, want a type error", err)
	}
	if _, err := tmplMul(nil, 1); err == nil {
		t.Error("mul with nil = nil error, want a type error")
	}
}

// TPL-004: toJSON emits a value once (not double-encoded), safe in a <script>.
func TestToJSON(t *testing.T) {
	out, err := tmplToJSON(map[string]interface{}{"a": 1, "b": []string{"x"}, "html": "</script>"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if len(s) == 0 || s[0] != '{' { // an object, not a quoted string
		t.Errorf("toJSON double-encoded: %s", s)
	}
	if strings.Contains(s, "</script>") { // must be \u-escaped
		t.Errorf("toJSON did not escape </script>: %s", s)
	}
}
