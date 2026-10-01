package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// provider builds a request for one chunk and decodes the response to MP3.
type provider interface {
	newRequest(ctx context.Context, cfg Config, text, lang string) (*http.Request, error)
	decode(body []byte) ([]byte, error)
	maxChars() int
}

// providerFor picks and checks the provider a config names.
func providerFor(cfg Config) (provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "generic":
		if cfg.APIURL == "" {
			return nil, fmt.Errorf("tts: provider generic needs api_url")
		}
		return generic{}, nil
	case "openai":
		return openAI{}, nil
	case "elevenlabs":
		if cfg.Voice == "" {
			return nil, fmt.Errorf("tts: provider elevenlabs needs a voice id")
		}
		return elevenLabs{}, nil
	case "google":
		return google{}, nil
	}
	return nil, fmt.Errorf("tts: unknown provider %q (generic, openai, elevenlabs, google)", cfg.Provider)
}

// jsonRequest is a POST of body as JSON with the given headers.
func jsonRequest(ctx context.Context, endpoint string, body any, headers map[string]string) (*http.Request, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ssg-tts")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	return req, nil
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// bearer is an Authorization value, or "" without a key.
func bearer(key string) string {
	if key == "" {
		return ""
	}
	return "Bearer " + key
}

// generic is ssg's own contract, which services/tts-server implements:
// POST {text, voice, lang, speed, format} → audio/mpeg.
type generic struct{}

func (generic) maxChars() int { return 20000 }

func (generic) newRequest(ctx context.Context, cfg Config, text, lang string) (*http.Request, error) {
	body := map[string]any{"text": text, "voice": cfg.Voice, "lang": lang, "format": "mp3"}
	if cfg.Speed > 0 {
		body["speed"] = cfg.Speed
	}
	return jsonRequest(ctx, cfg.APIURL, body, map[string]string{"Authorization": bearer(cfg.APIKey)})
}

func (generic) decode(body []byte) ([]byte, error) { return body, nil }

// openAI is POST /v1/audio/speech; input is capped at 4096 characters.
type openAI struct{}

func (openAI) maxChars() int { return 4000 }

func (openAI) newRequest(ctx context.Context, cfg Config, text, _ string) (*http.Request, error) {
	body := map[string]any{
		"model": orDefault(cfg.Model, "gpt-4o-mini-tts"), "input": text,
		"voice": orDefault(cfg.Voice, "alloy"), "response_format": "mp3",
	}
	if cfg.Speed > 0 {
		body["speed"] = cfg.Speed
	}
	if cfg.Instructions != "" {
		body["instructions"] = cfg.Instructions
	}
	endpoint := orDefault(cfg.APIURL, "https://api.openai.com/v1/audio/speech")
	return jsonRequest(ctx, endpoint, body, map[string]string{"Authorization": bearer(cfg.APIKey)})
}

func (openAI) decode(body []byte) ([]byte, error) { return body, nil }

// elevenLabs is POST /v1/text-to-speech/{voice_id}?output_format=mp3_44100_128.
type elevenLabs struct{}

func (elevenLabs) maxChars() int { return 4500 }

func (elevenLabs) newRequest(ctx context.Context, cfg Config, text, lang string) (*http.Request, error) {
	body := map[string]any{"text": text, "model_id": orDefault(cfg.Model, "eleven_multilingual_v2")}
	if code, _, _ := strings.Cut(lang, "-"); code != "" {
		body["language_code"] = strings.ToLower(code)
	}
	if cfg.Speed > 0 {
		body["voice_settings"] = map[string]any{"speed": cfg.Speed}
	}
	base := strings.TrimRight(orDefault(cfg.APIURL, "https://api.elevenlabs.io/v1/text-to-speech"), "/")
	endpoint := base + "/" + url.PathEscape(cfg.Voice) + "?output_format=mp3_44100_128"
	return jsonRequest(ctx, endpoint, body, map[string]string{"xi-api-key": cfg.APIKey, "Accept": "audio/mpeg"})
}

func (elevenLabs) decode(body []byte) ([]byte, error) { return body, nil }

// google is POST v1/text:synthesize; the MP3 comes back base64 in JSON, and a
// request carries at most 5000 bytes of input.
type google struct{}

func (google) maxChars() int { return 2400 } // bytes, not runes, are capped: leave room for UTF-8

func (google) newRequest(ctx context.Context, cfg Config, text, lang string) (*http.Request, error) {
	voice := map[string]any{"languageCode": orDefault(lang, "en-US")}
	if cfg.Voice != "" {
		voice["name"] = cfg.Voice
	}
	audio := map[string]any{"audioEncoding": "MP3"}
	if cfg.Speed > 0 {
		audio["speakingRate"] = cfg.Speed
	}
	body := map[string]any{"input": map[string]string{"text": text}, "voice": voice, "audioConfig": audio}
	endpoint := orDefault(cfg.APIURL, "https://texttospeech.googleapis.com/v1/text:synthesize")
	return jsonRequest(ctx, endpoint, body, map[string]string{"X-Goog-Api-Key": cfg.APIKey})
}

func (google) decode(body []byte) ([]byte, error) {
	var resp struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tts: google response: %w", err)
	}
	if resp.AudioContent == "" {
		return nil, fmt.Errorf("tts: google response has no audioContent")
	}
	return base64.StdEncoding.DecodeString(resp.AudioContent)
}
