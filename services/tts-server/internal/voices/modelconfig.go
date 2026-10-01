package voices

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PiperConfig is the subset of a Piper <model>.onnx.json the server needs.
type PiperConfig struct {
	Audio struct {
		SampleRate int    `json:"sample_rate"`
		Quality    string `json:"quality"`
	} `json:"audio"`
	Espeak struct {
		Voice string `json:"voice"`
	} `json:"espeak"`
	Language struct {
		Code string `json:"code"`
	} `json:"language"`
	Inference struct {
		LengthScale float64 `json:"length_scale"`
	} `json:"inference"`
	Dataset      string                     `json:"dataset"`
	PhonemeIDMap map[string]json.RawMessage `json:"phoneme_id_map"`
}

// ErrBadModelConfig marks a Piper config that cannot drive synthesis.
var ErrBadModelConfig = errors.New("invalid piper model config")

// ParsePiperConfig decodes and validates a Piper voice config.
func ParsePiperConfig(b []byte) (PiperConfig, error) {
	var c PiperConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%w: %w", ErrBadModelConfig, err)
	}
	if c.Audio.SampleRate < 8000 || c.Audio.SampleRate > 96000 {
		return c, fmt.Errorf("%w: audio.sample_rate %d out of range", ErrBadModelConfig, c.Audio.SampleRate)
	}
	if len(c.PhonemeIDMap) == 0 {
		return c, fmt.Errorf("%w: empty phoneme_id_map", ErrBadModelConfig)
	}
	if !ValidLang(c.Lang()) {
		return c, fmt.Errorf("%w: no usable language.code or espeak.voice", ErrBadModelConfig)
	}
	return c, nil
}

// Lang is the voice language: language.code, falling back to espeak.voice.
func (c PiperConfig) Lang() string {
	if c.Language.Code != "" {
		return NormalizeLang(c.Language.Code)
	}
	return NormalizeLang(c.Espeak.Voice)
}

// lengthScale is the model's default phoneme length, 1 when unset.
func (c PiperConfig) lengthScale() float64 {
	if c.Inference.LengthScale > 0 {
		return c.Inference.LengthScale
	}
	return 1
}
