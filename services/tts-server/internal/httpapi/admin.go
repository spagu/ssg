package httpapi

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

const (
	maxConfigBytes = 4 << 20          // Piper .onnx.json files are a few KiB
	uploadDeadline = 10 * time.Minute // read deadline for a voice upload
	onnxMagic      = 0x08             // ONNX protobuf: field 1 (ir_version), varint
)

// upload collects the staged parts of a voice upload.
type upload struct {
	id, model, config string
}

// uploadVoice handles POST /v1/voices (multipart: model, config, optional id).
func (a *api) uploadVoice(w http.ResponseWriter, r *http.Request) {
	if !a.PiperReady() {
		writeError(w, http.StatusServiceUnavailable, codeNotReady, "piper is not installed; uploaded voices could not be used")
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadDeadline))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadDeadline + time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, a.MaxUploadBytes+maxConfigBytes+64<<10)
	up, err := a.readUpload(r)
	defer a.Store.Discard(up.model, up.config)
	if err == nil {
		err = a.validateUpload(up)
	}
	if err == nil {
		err = a.installUpload(up)
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	var v voices.Voice
	for _, candidate := range a.Registry.Catalog().List() { // List fills in Default
		if candidate.ID == up.id {
			v = candidate
		}
	}
	w.Header().Set("Location", "/v1/voices?engine=piper")
	writeJSON(w, http.StatusCreated, v)
}

// readUpload streams the multipart parts into staged files.
func (a *api) readUpload(r *http.Request) (upload, error) {
	var up upload
	mr, err := r.MultipartReader()
	if err != nil {
		return up, badRequest(codeBadRequest, "expected multipart/form-data: %v", err)
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return up, nil
		}
		if err != nil {
			return up, decodeError(err)
		}
		if err := a.readPart(part, &up); err != nil {
			return up, err
		}
	}
}

func (a *api) readPart(part *multipart.Part, up *upload) error {
	var err error
	switch part.FormName() {
	case "id":
		var b []byte
		b, err = io.ReadAll(io.LimitReader(part, 128))
		up.id = strings.TrimSpace(string(b))
	case "model":
		if up.model != "" {
			return badRequest(codeBadRequest, "model given twice")
		}
		if up.id == "" {
			up.id = strings.TrimSuffix(path.Base(part.FileName()), ".onnx")
		}
		up.model, err = a.Store.Stage(part, a.MaxUploadBytes)
	case "config":
		if up.config != "" {
			return badRequest(codeBadRequest, "config given twice")
		}
		up.config, err = a.Store.Stage(part, maxConfigBytes)
	default:
		return badRequest(codeBadRequest, "unexpected form field %q (use model, config, id)", part.FormName())
	}
	return uploadError(err)
}

// uploadError maps staging failures to client errors.
func uploadError(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, voices.ErrTooLarge), errors.As(err, &tooBig):
		return &reqError{status: http.StatusRequestEntityTooLarge, code: codeTooLarge, msg: "upload too large"}
	}
	return err
}

// validateUpload checks the id, the config JSON and the model's first byte.
func (a *api) validateUpload(up upload) error {
	if up.model == "" || up.config == "" {
		return badRequest(codeBadRequest, "both model (.onnx) and config (.onnx.json) files are required")
	}
	if !voices.ValidID(up.id) {
		return badRequest(codeBadRequest, "invalid voice id %q: use 1-64 of [a-zA-Z0-9_.-], not starting with '.'", up.id)
	}
	cfg, err := a.Store.ReadStaged(up.config, maxConfigBytes)
	if err != nil {
		return err
	}
	if _, err := voices.ParsePiperConfig(cfg); err != nil {
		return badRequest(codeBadRequest, "%v", err)
	}
	head, err := a.Store.Head(up.model, 1)
	if err != nil {
		return err
	}
	if len(head) == 0 || head[0] != onnxMagic {
		return badRequest(codeBadRequest, "model does not look like an ONNX file")
	}
	return nil
}

// installUpload moves the files into place and reloads the registry.
func (a *api) installUpload(up upload) error {
	err := a.Store.Install(up.id, up.model, up.config)
	if errors.Is(err, voices.ErrVoiceExists) {
		return &reqError{status: http.StatusConflict, code: codeConflict, msg: "voice " + up.id + " already exists; delete it first"}
	}
	if err != nil {
		return err
	}
	a.Logger.Info("voice uploaded", "id", up.id)
	return a.Registry.Reload()
}

// deleteVoice handles DELETE /v1/voices/{id}.
func (a *api) deleteVoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, ok := a.Registry.Catalog().Get(id)
	switch {
	case !ok:
		writeError(w, http.StatusNotFound, codeNotFound, "no voice "+id)
		return
	case !v.Removable:
		writeError(w, http.StatusBadRequest, codeBadRequest, "voice "+id+" is built in or declared in voices.yaml with custom paths; it cannot be deleted over the API")
		return
	}
	if err := a.Store.Remove(id); err != nil {
		a.fail(w, err)
		return
	}
	a.Logger.Info("voice deleted", "id", id)
	if err := a.Registry.Reload(); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
