package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/cache"
	"github.com/spagu/ssg/services/tts-server/internal/engine"
	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/synth"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

const piperConfig = `{"audio":{"sample_rate":22050,"quality":"medium"},"language":{"code":"pl_PL"},"phoneme_id_map":{"_":[0]}}`

type fakeEngine struct {
	err   error
	block bool
}

func (f *fakeEngine) Available() bool { return true }
func (f *fakeEngine) MaxChunk() int   { return 100 }
func (f *fakeEngine) Synthesize(ctx context.Context, _ voices.Voice, _ string, _ float64) (audio.PCM, error) {
	if f.block {
		<-ctx.Done()
		return audio.PCM{}, ctx.Err()
	}
	return audio.PCM{SampleRate: 22050, Channels: 1, Data: []byte{1, 2, 3, 4}}, f.err
}

type fakeEncoder struct{}

func (fakeEncoder) Encode(_ context.Context, p audio.PCM, f audio.Format) ([]byte, error) {
	if f == audio.FormatWAV {
		return p.WAV()
	}
	return append([]byte("ID3"), p.Data...), nil
}

type harness struct {
	t        *testing.T
	h        http.Handler
	dir      string
	engine   *fakeEngine
	deps     Deps
	readyErr error
	piper    bool
}

type option func(*Deps)

func newHarness(t *testing.T, opts ...option) *harness {
	t.Helper()
	dir := t.TempDir()
	_ = os.WriteFile(dir+"/gosia.onnx", []byte{8, 1}, 0o600)
	_ = os.WriteFile(dir+"/gosia.onnx.json", []byte(piperConfig), 0o600)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	src := voices.Source{Root: root, Dir: dir, Piper: true, Logger: log, Builtins: []voices.Voice{
		{ID: "espeak:en", Engine: voices.EngineEspeak, Lang: "en", EspeakVoice: "en", Params: voices.Params{Speed: 1}},
	}}
	reg, err := voices.NewRegistry(src.Load)
	if err != nil {
		t.Fatal(err)
	}
	hs := &harness{t: t, dir: dir, engine: &fakeEngine{}, piper: true}
	svc := &synth.Service{
		Registry: reg, Encoder: fakeEncoder{}, Cache: cache.NewMemory(1 << 20),
		Engines: map[string]engine.Engine{voices.EngineEspeak: hs.engine, voices.EnginePiper: hs.engine},
		Gate:    limit.NewGate(1, 0, 10*time.Millisecond), Timeout: time.Second, DefaultLang: "en",
	}
	hs.deps = Deps{
		Synth: svc, Registry: reg, Store: &voices.Store{Root: root}, Logger: log,
		Ready:      func() error { return hs.readyErr },
		PiperReady: func() bool { return hs.piper },
		AdminKeys:  []string{"admin"}, MaxChars: 50, MaxUploadBytes: 1024,
	}
	for _, o := range opts {
		o(&hs.deps)
	}
	hs.h = NewHandler(hs.deps)
	return hs
}

func (hs *harness) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	return hs.send(req, headers...)
}

func (hs *harness) send(req *http.Request, headers ...string) *httptest.ResponseRecorder {
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec
}

// multipartBody builds an upload; parts maps field -> [filename, content].
func multipartBody(t *testing.T, parts [][3]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		var w io.Writer
		var err error
		if p[1] == "" {
			w, err = mw.CreateFormField(p[0])
		} else {
			w, err = mw.CreateFormFile(p[0], p[1])
		}
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, p[2])
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

var errBoom = errors.New("boom")
