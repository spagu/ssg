package models

// SlugifyTranslit against the shared table (testdata/slug-translit.json) that
// the editor-ui worker reads too, and against Slugify where the two must agree.

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"
)

func TestSlugifyTranslitSharedTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/slug-translit.json")
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Phrases  [][2]string          `json:"phrases"`
		RawRunes [][2]json.RawMessage `json:"runes"` // [code point, slug]
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Phrases) < 30 {
		t.Fatalf("only %d rows in the shared table", len(table.Phrases))
	}
	for _, row := range table.Phrases {
		if got := SlugifyTranslit(row[0]); got != row[1] {
			t.Errorf("SlugifyTranslit(%q) = %q, want %q", row[0], got, row[1])
		}
	}
	// Every code point: the ones listed slug as listed, every other one to "".
	kept := map[rune]string{}
	for _, pair := range table.RawRunes {
		var cp rune
		var slug string
		if json.Unmarshal(pair[0], &cp) != nil || json.Unmarshal(pair[1], &slug) != nil {
			t.Fatalf("bad runes row %s", pair)
		}
		kept[cp] = slug
	}
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if got := SlugifyTranslit(string(r)); got != kept[r] {
			t.Fatalf("U+%04X slugs to %q, the table says %q — regenerate it", r, got, kept[r])
		}
	}
}

func TestSlugifyTranslitKeepsPlainSlugsAndOtherScripts(t *testing.T) {
	// An ASCII label slugs the same either way: turning the switch on moves
	// only the URLs whose labels have letters the plain rule dropped.
	for _, s := range []string{"Hello, World!", "Go 1.27 release", "snake_case", ""} {
		if SlugifyTranslit(s) != Slugify(s) {
			t.Errorf("%q: %q vs %q", s, SlugifyTranslit(s), Slugify(s))
		}
	}
	// Letters of other scripts are not romanised, only passed to the filter.
	if got := TransliterateLatin("Привет é"); got != "привет e" {
		t.Errorf("TransliterateLatin = %q", got)
	}
}

func TestSlugFunc(t *testing.T) {
	if SlugFunc(false)("Straße") != "stra-e" || SlugFunc(true)("Straße") != "strasse" {
		t.Error("SlugFunc does not pick the rule by the switch")
	}
}
