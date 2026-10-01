package synth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/cache"
	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// fakeEngine returns 4 bytes of PCM per chunk, optionally failing or blocking.
type fakeEngine struct {
	calls int
	err   error
	rate  func(call int) int
	block bool
}

func (f *fakeEngine) Available() bool { return true }
func (f *fakeEngine) MaxChunk() int   { return 10 }
func (f *fakeEngine) Synthesize(ctx context.Context, _ voices.Voice, _ string, _ float64) (audio.PCM, error) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return audio.PCM{}, ctx.Err()
	}
	rate := 22050
	if f.rate != nil {
		rate = f.rate(f.calls)
	}
	return audio.PCM{SampleRate: rate, Channels: 1, Data: make([]byte, 4)}, f.err
}

type fakeEncoder struct{ err error }

func (f fakeEncoder) Encode(_ context.Context, p audio.PCM, _ audio.Format) ([]byte, error) {
	return p.Data, f.err
}

func newService(t *testing.T, eng *fakeEngine) *Service {
	t.Helper()
	reg, err := voices.NewRegistry(func() (voices.Catalog, error) {
		return catalogOf(
			voices.Voice{ID: "espeak:pl", Engine: voices.EngineEspeak, Lang: "pl"},
			voices.Voice{ID: "espeak:en-us", Engine: voices.EngineEspeak, Lang: "en-us"},
			voices.Voice{ID: "ghost", Engine: "nope", Lang: "xx"},
		), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		Registry: reg, Engines: map[string]engineIface{voices.EngineEspeak: eng},
		Encoder: fakeEncoder{}, Cache: cache.NewMemory(1 << 20),
		Gate: limit.NewGate(1, 0, 10*time.Millisecond), Timeout: time.Second, DefaultLang: "en",
	}
}

func TestPlan(t *testing.T) {
	s := newService(t, &fakeEngine{})
	base := Request{Text: "hi", Speed: 1, Format: audio.FormatMP3}
	cases := []struct {
		voice, lang, want string
		err               error
	}{
		{"espeak:pl", "", "espeak:pl", nil},
		{"", "pl-PL", "espeak:pl", nil},
		{"", "", "espeak:en-us", nil},
		{"missing", "", "", ErrUnknownVoice},
		{"", "fr", "", ErrUnknownLang},
	}
	for _, c := range cases {
		r := base
		r.Voice, r.Lang = c.voice, c.lang
		p, err := s.Plan(r)
		if !errors.Is(err, c.err) || p.Voice.ID != c.want {
			t.Errorf("%+v: %v %v", c, p.Voice.ID, err)
		}
	}
	a, _ := s.Plan(base)
	b := base
	b.Speed = 1.5
	bp, _ := s.Plan(b)
	if a.Key == bp.Key || len(a.Key) != 64 {
		t.Fatal("speed must change the key")
	}
}

func TestRender(t *testing.T) {
	eng := &fakeEngine{}
	s := newService(t, eng)
	p, _ := s.Plan(Request{Text: strings.Repeat("word ", 5), Voice: "espeak:pl", Speed: 1})
	res, err := s.Render(context.Background(), p)
	if err != nil || res.Cached || len(res.Audio) != 4*eng.calls || eng.calls < 2 {
		t.Fatal(err, res, eng.calls)
	}
	res, err = s.Render(context.Background(), p)
	if err != nil || !res.Cached {
		t.Fatal("second render must hit the cache")
	}
}

func TestRenderErrors(t *testing.T) {
	ctx := context.Background()
	s := newService(t, &fakeEngine{})
	ghost, _ := s.Plan(Request{Text: "x", Voice: "ghost"})
	if _, err := s.Render(ctx, ghost); !errors.Is(err, ErrNoEngine) {
		t.Fatal(err)
	}
	cases := map[string]*fakeEngine{
		"engine":   {err: errors.New("boom")},
		"mismatch": {rate: func(c int) int { return 8000 * c }},
	}
	for name, eng := range cases {
		s := newService(t, eng)
		p, _ := s.Plan(Request{Text: "aaaa bbbb cccc dddd", Voice: "espeak:pl"})
		if _, err := s.Render(ctx, p); err == nil {
			t.Error(name)
		}
	}
	s = newService(t, &fakeEngine{})
	s.Encoder = fakeEncoder{err: errors.New("lame")}
	p, _ := s.Plan(Request{Text: "x", Voice: "espeak:pl"})
	if _, err := s.Render(ctx, p); err == nil {
		t.Fatal("encoder error")
	}
}

func TestRenderTimeoutAndBusy(t *testing.T) {
	eng := &fakeEngine{block: true}
	s := newService(t, eng)
	s.Timeout = 20 * time.Millisecond
	p, _ := s.Plan(Request{Text: "x", Voice: "espeak:pl"})
	done := make(chan error)
	go func() { _, err := s.Render(context.Background(), p); done <- err }()
	time.Sleep(5 * time.Millisecond)
	p2, _ := s.Plan(Request{Text: "y", Voice: "espeak:pl"})
	if _, err := s.Render(context.Background(), p2); !errors.Is(err, limit.ErrBusy) {
		t.Fatal("want busy:", err)
	}
	if err := <-done; !errors.Is(err, ErrTimeout) {
		t.Fatal("want timeout:", err)
	}
}
