package voices

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the optional declaration file inside the voices directory.
const ManifestFile = "voices.yaml"

// Manifest is the voices.yaml document.
type Manifest struct {
	// Defaults maps a language tag to the voice id used for it.
	Defaults map[string]string `yaml:"defaults"`
	// Voices declares new voices or overrides discovered ones by id.
	Voices []ManifestVoice `yaml:"voices"`
}

// ManifestVoice is one entry of voices.yaml.
type ManifestVoice struct {
	ID          string  `yaml:"id"`
	Engine      string  `yaml:"engine"`
	Lang        string  `yaml:"lang"`
	Model       string  `yaml:"model"`       // piper: path relative to the voices dir (default <id>.onnx)
	Config      string  `yaml:"config"`      // piper: default <model>.json
	EspeakVoice string  `yaml:"espeakVoice"` // espeak: -v value (default lang), e.g. pl+f3
	Name        string  `yaml:"name"`
	Gender      string  `yaml:"gender"`
	Quality     string  `yaml:"quality"`
	Speed       float64 `yaml:"speed"`
	Pitch       int     `yaml:"pitch"`
	Speaker     int     `yaml:"speaker"`
}

// ParseManifest strictly decodes voices.yaml: unknown keys are errors, so a
// typo never silently falls back to defaults.
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		return m, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	for i, v := range m.Voices {
		if err := v.validate(); err != nil {
			return m, fmt.Errorf("%s: voices[%d]: %w", ManifestFile, i, err)
		}
	}
	for lang, id := range m.Defaults {
		if !ValidLang(lang) || id == "" {
			return m, fmt.Errorf("%s: defaults: bad entry %q: %q", ManifestFile, lang, id)
		}
	}
	return m, nil
}

func (v ManifestVoice) validate() error {
	switch {
	case !ValidID(strings.TrimPrefix(v.ID, "espeak:")):
		return fmt.Errorf("invalid id %q", v.ID)
	case v.Engine != EnginePiper && v.Engine != EngineEspeak:
		return fmt.Errorf("engine must be %q or %q", EnginePiper, EngineEspeak)
	case v.Lang != "" && !ValidLang(v.Lang):
		return fmt.Errorf("invalid lang %q", v.Lang)
	case v.Engine == EngineEspeak && v.Lang == "":
		return errors.New("espeak voices need lang")
	case v.EspeakVoice != "" && !validEspeakVoice(v.EspeakVoice):
		return fmt.Errorf("invalid espeakVoice %q", v.EspeakVoice)
	case v.Speed < 0 || v.Speed > 4:
		return errors.New("speed must be between 0 and 4")
	case v.Pitch < 0 || v.Pitch > 99:
		return errors.New("pitch must be between 0 and 99")
	case v.Speaker < 0:
		return errors.New("speaker must be >= 0")
	}
	return nil
}

// apply copies the declared fields onto v (zero values keep v's own).
func (v ManifestVoice) apply(base Voice) Voice {
	base.ID, base.Engine = v.ID, v.Engine
	base.Lang = firstNonEmpty(NormalizeLang(v.Lang), base.Lang)
	base.Name = firstNonEmpty(v.Name, base.Name)
	base.Gender = firstNonEmpty(v.Gender, base.Gender)
	base.Quality = firstNonEmpty(v.Quality, base.Quality)
	if v.Speed > 0 {
		base.Params.Speed = v.Speed
	}
	if v.Pitch > 0 {
		base.Params.Pitch = v.Pitch
	}
	if v.Speaker > 0 {
		base.Params.Speaker = v.Speaker
	}
	return base
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
