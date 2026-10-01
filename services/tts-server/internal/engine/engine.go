// Package engine adapts speech synthesizers (espeak-ng, Piper) to a common
// interface. New engines plug in by implementing Engine and being registered
// under the name used in voices.Voice.Engine.
package engine

import (
	"context"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// Engine synthesizes one chunk of plain text with one voice.
type Engine interface {
	// Available reports whether the engine's binary is installed.
	Available() bool
	// MaxChunk is the longest text, in characters, one Synthesize call should get.
	MaxChunk() int
	// Synthesize renders text. speed multiplies the voice's default speed.
	Synthesize(ctx context.Context, v voices.Voice, text string, speed float64) (audio.PCM, error)
}
