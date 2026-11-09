package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestStorySayShared: the cases the browser's story-say.js passes too.
func TestStorySayShared(t *testing.T) {
	var fixture struct {
		Rules map[string]string `json:"rules"`
		Cases [][2]string       `json:"cases"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "story", "pronounce.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	s := newSayer(fixture.Rules, "pl")
	for _, c := range fixture.Cases {
		if got := s.Say(c[0]); got != c[1] {
			t.Errorf("%q = %q, want %q", c[0], got, c[1])
		}
	}
}

// TestStorySayBuiltinSets: every built-in set reads its own examples.
func TestStorySayBuiltinSets(t *testing.T) {
	names := pronounceSets()
	if len(names) < 2 {
		t.Fatalf("sets = %v", names)
	}
	for _, name := range names {
		set, err := loadPronounceSet(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(set.Examples) == 0 {
			t.Errorf("%s has no examples", name)
		}
		s := newSayer(set.Rules, set.Lang)
		for _, e := range set.Examples {
			if got := s.Say(e[0]); got != e[1] {
				t.Errorf("%s: %q = %q, want %q", name, e[0], got, e[1])
			}
		}
	}
	if _, err := loadPronounceSet("missing"); err == nil {
		t.Error("a missing set loads")
	}
}

// TestStorySayEdges: no rules, an empty key, English plurals, words only
// in Polish, a form with no number before it, numbers in words.
func TestStorySayEdges(t *testing.T) {
	if got := newSayer(nil, "pl").Say("100g"); got != "100g" {
		t.Errorf("no rules = %q", got)
	}
	if got := newSayer(map[string]string{" ": "x", "{num} g": "{num} gram|grams"}, "en-GB").Say("1 g, 2 g, 1.5 g"); got != "1 gram, 2 grams, 1.5 grams" {
		t.Errorf("en = %q", got)
	}
	if got := newSayer(map[string]string{"{num} cup|cups": "{num:one|two} {word}"}, "en").Say("2 cups"); got != "2 cups" {
		t.Errorf("words only in Polish = %q", got)
	}
	if got := newSayer(map[string]string{"szt.": "sztuka|sztuki"}, "pl").Say("3 szt."); got != "3 sztuka" {
		t.Errorf("no number = %q", got)
	}
	if got := newSayer(map[string]string{"{num} x": "{num} {num} a|b|c|d"}, "pl").Say("5 x"); got != "5  c" {
		t.Errorf("a second {num} with no number = %q", got)
	}
	s := newSayer(map[string]string{"{num} x": "{num:jedna|dwie}"}, "pl")
	for in, want := range map[string]string{"10 x": "dziesięć", "300 x": "trzysta", "12002 x": "dwanaście tysięcy dwie",
		"1000 x": "tysiąc", "0 x": "zero", "1,5 x": "1,5", "1234567 x": "1234567", "1 x": "jedna"} {
		if got := s.Say(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
	if got := newSayer(map[string]string{"{num} g": "{num} gram|gramy"}, "pl").Say("1,5 g"); got != "1,5 gramy" {
		t.Errorf("a missing form falls back to the last = %q", got)
	}
	if _, ok := compileSay("{num} ("); !ok {
		t.Error("special characters are literal")
	}
}
