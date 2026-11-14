package admintheme

import (
	"regexp"
	"strings"
	"testing"
)

func TestStack(t *testing.T) {
	for in, want := range map[string]string{
		"Georgia, serif": `"Georgia", "Noto Serif", "Noto Serif Devanagari", "Nirmala UI", serif`,
		`"Brand Serif", 'Iowan Old Style', Palatino Linotype, Serif`: `"Brand Serif", "Iowan Old Style", "Palatino Linotype", "Noto Serif", "Noto Serif Devanagari", "Nirmala UI", serif`,
		"Georgia":                    `"Georgia", "Noto Serif", "Noto Serif Devanagari", "Nirmala UI", serif`,
		"Georgia, Georgia, serif, x": `"Georgia", "Noto Serif", "Noto Serif Devanagari", "Nirmala UI", serif, "x"`,
		`"Noto Serif Devanagari", "Nirmala UI", "Noto Serif", ui-serif`: `"Noto Serif Devanagari", "Nirmala UI", "Noto Serif", ui-serif`,
		`"Zażółć Grotesk", sans-serif`:                                  `"Zażółć Grotesk", "Noto Sans", "Noto Sans Devanagari", "Nirmala UI", sans-serif`,
	} {
		generic := "serif"
		if strings.Contains(in, "sans") {
			generic = "sans-serif"
		}
		got, err := Stack(in, generic)
		if err != nil || got != want {
			t.Errorf("%s\n got %s %v\nwant %s", in, got, err, want)
		}
	}
}

// hostile are values written to break out of a CSS declaration, a CSS
// string, a JSON string or an HTML attribute.
var hostile = []string{
	"Georgia; } body { display: none", "Georgia}", "Georgia{", `"Georgia"; color: red`, "url(https://example.com/x.woff2)",
	"@import 'x'", "Georgia /* c */", "Georgia\\3b", "Georgia\nserif", "Georgia\r", `"Geo"rgia"`, `'Geo"`, `"Geo'`,
	"inherit", "initial", "1Georgia", "Georgia,", ",serif", "", "  ", `"a,b"`, "<script>", "Georgia</style>", "expression(alert(1))",
	"Georgia\x00", "Georgia ", strings.Repeat("a", 301), `"` + strings.Repeat("a", 65) + `"`, "Ge orgia;", "--x", "-Georgia",
}

var safeStack = regexp.MustCompile(`^[\p{L}\p{N} ."_,-]+$`)

func TestStackRefusesInjection(t *testing.T) {
	for _, in := range hostile {
		got, err := Stack(in, "serif")
		if err == nil {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}

// FuzzStack: whatever comes in, what comes out is a list of quoted names and
// generic keywords — nothing that ends a declaration, a block or a string.
func FuzzStack(f *testing.F) {
	for _, s := range append(hostile, "Georgia, serif", `"Brand", system-ui`) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := Stack(in, "sans-serif")
		if err != nil {
			return
		}
		if !safeStack.MatchString(got) || strings.Count(got, `"`)%2 != 0 || strings.Contains(got, `""`) {
			t.Fatalf("%q → %q", in, got)
		}
	})
}

func TestFontFileParts(t *testing.T) {
	if n, err := FamilyName(" 'Brand Serif' "); err != nil || n != "Brand Serif" {
		t.Errorf("%q %v", n, err)
	}
	for _, bad := range []string{"serif", "A, B", "x;y", ""} {
		if _, err := FamilyName(bad); err == nil {
			t.Errorf("family %q", bad)
		}
	}
	for in, want := range map[string]string{"": "normal", "bold": "bold", "400": "400", "400 700": "400 700", "1000": "1000"} {
		if got, err := Weight(in); err != nil || got != want {
			t.Errorf("weight %q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"0", "1001", "400  700", "400;", "lighter", "4000"} {
		if _, err := Weight(bad); err == nil {
			t.Errorf("weight %q", bad)
		}
	}
	for in, want := range map[string]string{"": "normal", "italic": "italic", "oblique": "oblique", "normal": "normal"} {
		if got, err := Style(in); err != nil || got != want {
			t.Errorf("style %q", in)
		}
	}
	if _, err := Style("oblique 10deg"); err == nil {
		t.Error("style with an angle")
	}
}
