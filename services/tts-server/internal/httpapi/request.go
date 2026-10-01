package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/synth"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// Accepted speed multiplier range.
const (
	minSpeed = 0.25
	maxSpeed = 4.0
)

// speechBody is the JSON body of POST /v1/speech.
type speechBody struct {
	Text   string   `json:"text"`
	Voice  string   `json:"voice"`
	Lang   string   `json:"lang"`
	Speed  *float64 `json:"speed"`
	Format string   `json:"format"`
}

// reqError is a client error with its HTTP status and code.
type reqError struct {
	status int
	code   string
	msg    string
}

func (e *reqError) Error() string { return e.msg }

func badRequest(code, format string, args ...any) *reqError {
	return &reqError{status: http.StatusBadRequest, code: code, msg: fmt.Sprintf(format, args...)}
}

// decodeSpeech strictly decodes and validates a speech request. The body is
// capped at four bytes per allowed character plus room for the other fields.
func (a *api) decodeSpeech(w http.ResponseWriter, r *http.Request) (synth.Request, error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(a.MaxChars)*4+4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body speechBody
	if err := dec.Decode(&body); err != nil {
		return synth.Request{}, decodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return synth.Request{}, badRequest(codeBadRequest, "body must hold a single JSON object")
	}
	return body.validate(a.MaxChars)
}

func decodeError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return &reqError{status: http.StatusRequestEntityTooLarge, code: codeTooLarge,
			msg: fmt.Sprintf("request body larger than %d bytes", tooBig.Limit)}
	}
	return badRequest(codeBadRequest, "invalid JSON: %v", err)
}

// validate checks every field and applies defaults. (encoding/json already
// replaced invalid UTF-8 with U+FFFD, so text is valid UTF-8 here.)
func (b speechBody) validate(maxChars int) (synth.Request, error) {
	text := strings.TrimSpace(b.Text)
	switch n := utf8.RuneCountInString(text); {
	case n == 0:
		return synth.Request{}, badRequest(codeBadRequest, "text is required")
	case n > maxChars:
		return synth.Request{}, badRequest(codeTextTooLong, "text has %d characters; the limit is %d", n, maxChars)
	}
	if b.Lang != "" && !voices.ValidLang(b.Lang) {
		return synth.Request{}, badRequest(codeUnknownLang, "lang %q is not a BCP-47 tag", b.Lang)
	}
	speed := 1.0
	if b.Speed != nil {
		speed = *b.Speed
	}
	if speed < minSpeed || speed > maxSpeed {
		return synth.Request{}, badRequest(codeBadRequest, "speed must be between %.2f and %.1f", minSpeed, maxSpeed)
	}
	format, err := audio.ParseFormat(b.Format)
	if err != nil {
		return synth.Request{}, badRequest(codeBadRequest, "%v", err)
	}
	return synth.Request{Text: text, Voice: strings.TrimSpace(b.Voice), Lang: b.Lang, Speed: speed, Format: format}, nil
}

// etagMatches implements If-None-Match: a list of (possibly weak) tags or *.
func etagMatches(header, etag string) bool {
	for tag := range strings.SplitSeq(header, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == "*" || tag == etag {
			return true
		}
	}
	return false
}
