// Package synth turns a validated speech request into encoded audio: it
// resolves the voice, consults the cache, waits for an engine slot, renders
// the text chunk by chunk and encodes the result.
package synth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/cache"
	"github.com/spagu/ssg/services/tts-server/internal/engine"
	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// Errors a caller maps to HTTP statuses.
var (
	ErrUnknownVoice = errors.New("unknown voice")
	ErrUnknownLang  = errors.New("no voice for language")
	ErrNoEngine     = errors.New("engine not available")
	ErrTimeout      = errors.New("synthesis timed out")
)

// Request is a validated speech request.
type Request struct {
	Text   string
	Voice  string
	Lang   string
	Speed  float64
	Format audio.Format
}

// Plan is a request bound to a concrete voice, with its cache key.
type Plan struct {
	Request
	Voice voices.Voice
	Key   string
}

// Result is rendered audio ready to serve.
type Result struct {
	Audio  []byte
	Cached bool
}

// AudioEncoder converts PCM into a container format.
type AudioEncoder interface {
	Encode(ctx context.Context, pcm audio.PCM, f audio.Format) ([]byte, error)
}

// Service holds the collaborators of a synthesis.
type Service struct {
	Registry    *voices.Registry
	Engines     map[string]engine.Engine
	Encoder     AudioEncoder
	Cache       *cache.Cache
	Gate        *limit.Gate
	Timeout     time.Duration
	DefaultLang string
}

// Plan resolves the voice (explicit id, else the default for the language,
// else the default for DefaultLang) and computes the cache key, which also
// serves as the ETag.
func (s *Service) Plan(req Request) (Plan, error) {
	cat := s.Registry.Catalog()
	var v voices.Voice
	var ok bool
	switch {
	case req.Voice != "":
		if v, ok = cat.Get(req.Voice); !ok {
			return Plan{}, fmt.Errorf("%w: %q", ErrUnknownVoice, req.Voice)
		}
	default:
		lang := req.Lang
		if lang == "" {
			lang = s.DefaultLang
		}
		if v, ok = cat.Resolve(lang); !ok {
			return Plan{}, fmt.Errorf("%w: %q", ErrUnknownLang, lang)
		}
	}
	key := cache.Key(v.ID, v.Revision, v.Lang, strconv.FormatFloat(req.Speed, 'f', 3, 64), string(req.Format), req.Text)
	return Plan{Request: req, Voice: v, Key: key}, nil
}

// Render returns the audio for p, from cache when possible.
func (s *Service) Render(ctx context.Context, p Plan) (Result, error) {
	if b, ok := s.Cache.Get(p.Key); ok {
		return Result{Audio: b, Cached: true}, nil
	}
	eng, ok := s.Engines[p.Voice.Engine]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrNoEngine, p.Voice.Engine)
	}
	release, err := s.Gate.Acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	b, err := s.render(ctx, eng, p)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("%w after %s: %w", ErrTimeout, s.Timeout, err)
		}
		return Result{}, err
	}
	s.Cache.Put(p.Key, b)
	return Result{Audio: b}, nil
}

// render synthesizes every chunk, joins the PCM and encodes it once.
func (s *Service) render(ctx context.Context, eng engine.Engine, p Plan) ([]byte, error) {
	var pcm audio.PCM
	for _, chunk := range engine.Split(p.Text, eng.MaxChunk()) {
		part, err := eng.Synthesize(ctx, p.Voice, chunk, p.Speed)
		if err != nil {
			return nil, fmt.Errorf("synthesize: %w", err)
		}
		if pcm, err = pcm.Append(part); err != nil {
			return nil, err
		}
	}
	return s.Encoder.Encode(ctx, pcm, p.Format)
}
