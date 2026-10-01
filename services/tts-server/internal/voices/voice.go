// Package voices discovers, describes and stores the voices the server can
// speak with: Piper neural models from the voices directory, espeak-ng's
// built-in languages, and overrides declared in voices.yaml.
package voices

import (
	"regexp"
	"strings"
)

// Engine names used in Voice.Engine and voices.yaml.
const (
	EnginePiper  = "piper"
	EngineEspeak = "espeak"
)

// Voice is one selectable voice. Exported JSON fields form the public
// /v1/voices listing; the rest is internal wiring.
type Voice struct {
	ID         string `json:"id"`
	Engine     string `json:"engine"`
	Lang       string `json:"lang"`
	Name       string `json:"name,omitempty"`
	Gender     string `json:"gender,omitempty"`
	Quality    string `json:"quality,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Default    bool   `json:"default"`
	Params     Params `json:"defaults"`

	Model       string `json:"-"` // absolute path to the .onnx model (piper)
	Config      string `json:"-"` // absolute path to the .onnx.json config (piper)
	EspeakVoice string `json:"-"` // value passed to espeak-ng -v
	Revision    string `json:"-"` // changes when the model files change
	Removable   bool   `json:"-"` // backed by files in the voices directory

	// Aliases maps other language tags this voice serves to a priority
	// (lower is better), as espeak-ng reports in its "Other Languages" column.
	Aliases map[string]int `json:"-"`
}

// Params are the voice's default synthesis parameters.
type Params struct {
	Speed       float64 `json:"speed"`
	Pitch       int     `json:"pitch,omitempty"`
	Speaker     int     `json:"speaker,omitempty"`
	LengthScale float64 `json:"lengthScale,omitempty"`
}

var (
	idPattern     = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	langPattern   = regexp.MustCompile(`^[a-zA-Z]{2,8}([-_][a-zA-Z0-9]{1,8})*$`)
	espeakPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_+/-]{0,63}$`)
)

// ValidID reports whether id is safe to use as a file name in the voices
// directory: 1-64 characters from [a-zA-Z0-9_.-], not starting with a dot.
func ValidID(id string) bool {
	return idPattern.MatchString(id) && !strings.HasPrefix(id, ".")
}

// ValidLang reports whether s looks like a BCP-47 tag (en, pl, en-GB, en_US).
func ValidLang(s string) bool { return langPattern.MatchString(s) }

// validEspeakVoice rejects values that espeak-ng could read as an option.
func validEspeakVoice(s string) bool { return espeakPattern.MatchString(s) }

// NormalizeLang lower-cases a tag and uses '-' as separator: en_US -> en-us.
func NormalizeLang(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-"))
}

// primary returns the language subtag of a normalised tag: en-us -> en.
func primary(lang string) string {
	p, _, _ := strings.Cut(lang, "-")
	return p
}
