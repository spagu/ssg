package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/synth"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// busyRetryAfter is the Retry-After (seconds) sent with 503 busy responses.
const busyRetryAfter = "5"

// speech handles POST /v1/speech.
func (a *api) speech(w http.ResponseWriter, r *http.Request) {
	req, err := a.decodeSpeech(w, r)
	if err != nil {
		a.fail(w, err)
		return
	}
	plan, err := a.Synth.Plan(req)
	if err != nil {
		a.fail(w, err)
		return
	}
	etag := `"` + plan.Key + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "no-cache")
	h.Set("X-TTS-Voice", plan.Voice.ID)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	res, err := a.Synth.Render(r.Context(), plan)
	if err != nil {
		a.fail(w, err)
		return
	}
	h.Set("Content-Type", req.Format.ContentType())
	h.Set("Content-Length", strconv.Itoa(len(res.Audio)))
	h.Set("X-TTS-Cache", map[bool]string{true: "hit", false: "miss"}[res.Cached])
	_, _ = w.Write(res.Audio)
}

// fail maps an error to a status and code. Internal details are logged, not
// returned.
func (a *api) fail(w http.ResponseWriter, err error) {
	var re *reqError
	switch {
	case errors.As(err, &re):
		writeError(w, re.status, re.code, re.msg)
	case errors.Is(err, synth.ErrUnknownVoice):
		writeError(w, http.StatusBadRequest, codeUnknownVoice, err.Error())
	case errors.Is(err, synth.ErrUnknownLang):
		writeError(w, http.StatusBadRequest, codeUnknownLang, err.Error())
	case errors.Is(err, voices.ErrNoSuchVoice):
		writeError(w, http.StatusNotFound, codeNotFound, err.Error())
	case errors.Is(err, limit.ErrBusy):
		w.Header().Set("Retry-After", busyRetryAfter)
		writeError(w, http.StatusServiceUnavailable, codeBusy, "all engine slots are busy; retry later")
	case errors.Is(err, synth.ErrNoEngine):
		writeError(w, http.StatusServiceUnavailable, codeNotReady, err.Error())
	case errors.Is(err, synth.ErrTimeout):
		a.Logger.Warn("synthesis timeout", "err", err)
		writeError(w, http.StatusGatewayTimeout, codeTimeout, "synthesis timed out")
	case errors.Is(err, context.Canceled):
		a.Logger.Info("client went away")
	default:
		a.Logger.Error("synthesis failed", "err", err)
		writeError(w, http.StatusInternalServerError, codeEngineFailure, "speech synthesis failed")
	}
}
