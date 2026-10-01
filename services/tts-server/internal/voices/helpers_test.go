package voices

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

const goodConfig = `{"audio":{"sample_rate":22050,"quality":"medium"},"espeak":{"voice":"pl"},
"language":{"code":"pl_PL"},"inference":{"length_scale":1.1},"dataset":"gosia","phoneme_id_map":{"_":[0]}}`

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// voiceDir creates a temp voices dir with the given files and opens it.
func voiceDir(t *testing.T, files map[string]string) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

func espeakVoices() []Voice {
	return []Voice{
		{ID: "espeak:en-gb", Engine: EngineEspeak, Lang: "en-gb", EspeakVoice: "en-gb", Aliases: map[string]int{"en": 2}},
		{ID: "espeak:en-029", Engine: EngineEspeak, Lang: "en-029", EspeakVoice: "en-029", Aliases: map[string]int{"en": 10}},
		{ID: "espeak:en-us", Engine: EngineEspeak, Lang: "en-us", EspeakVoice: "en-us", Aliases: map[string]int{"en": 3}},
		{ID: "espeak:pl", Engine: EngineEspeak, Lang: "pl", EspeakVoice: "pl"},
		{ID: "espeak:de", Engine: EngineEspeak, Lang: "de", EspeakVoice: "de"},
	}
}
