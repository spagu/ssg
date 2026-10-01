package voices

import (
	"errors"
	"strings"
	"testing"
)

func load(t *testing.T, files map[string]string, piper bool) (Catalog, error) {
	t.Helper()
	dir, root := voiceDir(t, files)
	return Source{Root: root, Dir: dir, Builtins: espeakVoices(), Piper: piper, Logger: quietLogger()}.Load()
}

func TestDiscoverPiper(t *testing.T) {
	cat, err := load(t, map[string]string{
		"pl_PL-gosia-medium.onnx": "m", "pl_PL-gosia-medium.onnx.json": goodConfig,
		"broken.onnx": "m", "broken.onnx.json": "{", "orphan.onnx": "m", ".hidden.onnx": "m", "notes.txt": "x",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := cat.Get("pl_PL-gosia-medium")
	if !ok || v.Lang != "pl-pl" || v.SampleRate != 22050 || !v.Removable || !strings.HasSuffix(v.Model, ".onnx") || v.Revision == "" {
		t.Fatalf("%+v", v)
	}
	if _, ok := cat.Get("broken"); ok || cat.Len() != len(espeakVoices())+1 {
		t.Fatal("broken model must be skipped", cat.Len())
	}
	if d, _ := cat.Resolve("pl"); d.ID != v.ID {
		t.Fatal("piper should be pl default")
	}
}

func TestDiscoverWithoutPiper(t *testing.T) {
	cat, err := load(t, map[string]string{"x.onnx": "m", "x.onnx.json": goodConfig}, false)
	if err != nil || cat.Len() != len(espeakVoices()) {
		t.Fatal(err, cat.Len())
	}
}

func TestManifest(t *testing.T) {
	manifest := `
defaults:
  en: robot
voices:
  - id: robot
    engine: espeak
    lang: en-GB
    espeakVoice: en-gb+f3
    pitch: 70
    speed: 1.2
    gender: female
  - id: gosia
    engine: piper
    model: pl_PL-gosia-medium.onnx
    lang: pl
    speaker: 1
    name: Gosia
    quality: high
  - id: espeak:de
    engine: espeak
    lang: de
`
	cat, err := load(t, map[string]string{
		"voices.yaml": manifest, "pl_PL-gosia-medium.onnx": "m", "pl_PL-gosia-medium.onnx.json": goodConfig,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	robot, _ := cat.Get("robot")
	if robot.EspeakVoice != "en-gb+f3" || robot.Params.Pitch != 70 || robot.Params.Speed != 1.2 || robot.Gender != "female" || robot.Removable {
		t.Fatalf("%+v", robot)
	}
	g, _ := cat.Get("gosia")
	if g.Lang != "pl" || g.Params.Speaker != 1 || g.Name != "Gosia" || g.Quality != "high" || g.Removable {
		t.Fatalf("%+v", g)
	}
	if de, _ := cat.Get("espeak:de"); de.EspeakVoice != "de" || de.Params.Speed != 1 {
		t.Fatalf("%+v", de)
	}
	if d, _ := cat.Resolve("en-US"); d.ID != "robot" {
		t.Fatal("manifest default", d.ID)
	}
}

func TestManifestErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"syntax":          {"voices.yaml": "voices: [:"},
		"unknown key":     {"voices.yaml": "bogus: 1"},
		"bad id":          {"voices.yaml": "voices: [{id: '../x', engine: espeak, lang: en}]"},
		"bad engine":      {"voices.yaml": "voices: [{id: a, engine: say, lang: en}]"},
		"bad lang":        {"voices.yaml": "voices: [{id: a, engine: espeak, lang: '!!'}]"},
		"espeak no lang":  {"voices.yaml": "voices: [{id: a, engine: espeak}]"},
		"espeak voice":    {"voices.yaml": "voices: [{id: a, engine: espeak, lang: en, espeakVoice: '-x'}]"},
		"speed":           {"voices.yaml": "voices: [{id: a, engine: espeak, lang: en, speed: 9}]"},
		"pitch":           {"voices.yaml": "voices: [{id: a, engine: espeak, lang: en, pitch: 200}]"},
		"speaker":         {"voices.yaml": "voices: [{id: a, engine: espeak, lang: en, speaker: -1}]"},
		"bad default":     {"voices.yaml": "defaults: {'!': a}"},
		"unknown default": {"voices.yaml": "defaults: {en: nope}"},
		"missing model":   {"voices.yaml": "voices: [{id: a, engine: piper}]"},
		"escaping model":  {"voices.yaml": "voices: [{id: a, engine: piper, model: ../a.onnx}]"},
		"config too big":  {"voices.yaml": "voices: [{id: a, engine: piper}]", "a.onnx": "m", "a.onnx.json": strings.Repeat(" ", maxConfigBytes+1)},
		"no config":       {"voices.yaml": "voices: [{id: a, engine: piper}]", "a.onnx": "m"},
	}
	for name, files := range cases {
		if _, err := load(t, files, true); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := load(t, map[string]string{"voices.yaml": "voices: [{id: a, engine: piper}]"}, false); err == nil {
		t.Error("piper voice without piper must fail")
	}
	if _, err := load(t, map[string]string{"voices.yaml": ""}, true); err != nil {
		t.Error("empty manifest is fine:", err)
	}
}

func TestLoadUnreadable(t *testing.T) {
	_, root := voiceDir(t, nil)
	_ = root.Close()
	if _, err := (Source{Root: root, Logger: quietLogger()}).Load(); err == nil {
		t.Fatal("closed root must fail")
	}
	dir, root2 := voiceDir(t, nil)
	_ = dir
	if err := root2.Mkdir(ManifestFile, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Source{Root: root2, Logger: quietLogger()}).Load(); err == nil {
		t.Fatal("manifest directory must fail")
	}
}

func TestRegistry(t *testing.T) {
	calls := 0
	r, err := NewRegistry(func() (Catalog, error) {
		calls++
		if calls > 1 {
			return Catalog{}, errors.New("broken")
		}
		c := newCatalog()
		c.add(Voice{ID: "a"})
		return c, nil
	})
	if err != nil || r.Catalog().Len() != 1 {
		t.Fatal(err)
	}
	if err := r.Reload(); err == nil || r.Catalog().Len() != 1 {
		t.Fatal("failed reload must keep old catalog")
	}
}
