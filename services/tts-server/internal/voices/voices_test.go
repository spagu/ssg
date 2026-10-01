package voices

import (
	"errors"
	"strings"
	"testing"
)

func TestValidators(t *testing.T) {
	for id, want := range map[string]bool{"en_US-lessac-medium": true, "a.b": true, ".hidden": false,
		"../x": false, "": false, strings.Repeat("a", 65): false, "a b": false} {
		if ValidID(id) != want {
			t.Errorf("ValidID(%q)", id)
		}
	}
	for l, want := range map[string]bool{"en": true, "en-GB": true, "pl_PL": true, "e": false, "en--x": false} {
		if ValidLang(l) != want {
			t.Errorf("ValidLang(%q)", l)
		}
	}
	if NormalizeLang(" en_US ") != "en-us" || primary("en-us") != "en" {
		t.Error("normalize")
	}
	if validEspeakVoice("-x") || !validEspeakVoice("pl+f3") {
		t.Error("espeak voice")
	}
}

func TestParsePiperConfig(t *testing.T) {
	c, err := ParsePiperConfig([]byte(goodConfig))
	if err != nil || c.Lang() != "pl-pl" || c.lengthScale() != 1.1 {
		t.Fatal(err, c.Lang())
	}
	c.Language.Code = ""
	c.Inference.LengthScale = 0
	if c.Lang() != "pl" || c.lengthScale() != 1 {
		t.Fatal("fallbacks")
	}
	bad := []string{
		`{`,
		`{"audio":{"sample_rate":10},"phoneme_id_map":{"a":[1]},"language":{"code":"pl"}}`,
		`{"audio":{"sample_rate":22050},"language":{"code":"pl"}}`,
		`{"audio":{"sample_rate":22050},"phoneme_id_map":{"a":[1]}}`,
	}
	for _, b := range bad {
		if _, err := ParsePiperConfig([]byte(b)); !errors.Is(err, ErrBadModelConfig) {
			t.Errorf("%s: %v", b, err)
		}
	}
}

func TestResolve(t *testing.T) {
	cat := newCatalog()
	for _, v := range espeakVoices() {
		cat.add(v)
	}
	pick := func(lang string) string { v, _ := cat.Resolve(lang); return v.ID }
	if pick("en") != "espeak:en-gb" || pick("en-US") != "espeak:en-us" || pick("pl-PL") != "espeak:pl" {
		t.Fatal(pick("en"), pick("en-US"), pick("pl-PL"))
	}
	if _, ok := cat.Resolve("fr"); ok {
		t.Fatal("fr should not resolve")
	}
	cat.add(Voice{ID: "zz-piper", Engine: EnginePiper, Lang: "pl-pl"})
	cat.add(Voice{ID: "aa-piper", Engine: EnginePiper, Lang: "pl-pl"})
	if pick("pl") != "aa-piper" {
		t.Fatal("piper should win, ties by id:", pick("pl"))
	}
	cat.defaults["pl"] = "zz-piper"
	if pick("pl-PL") != "zz-piper" {
		t.Fatal("explicit default via primary subtag")
	}
	list := cat.List()
	if list[0].ID != "aa-piper" || list[0].Default || !list[1].Default || list[1].ID != "zz-piper" {
		t.Fatalf("%+v", list[:2])
	}
}

func TestCatalogOf(t *testing.T) {
	if c := CatalogOf(Voice{ID: "a"}, Voice{ID: "b"}); c.Len() != 2 {
		t.Fatal(c.Len())
	}
}
