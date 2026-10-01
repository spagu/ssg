package engine

import (
	"context"
	"strconv"
	"strings"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/execx"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// Piper drives the standalone Piper binary (rhasspy/piper release builds).
type Piper struct {
	Bin    string
	Runner execx.Runner
}

// Available reports whether the piper binary is installed.
func (p Piper) Available() bool { return execx.Available(p.Bin) }

// MaxChunk is the size Piper handles comfortably in a single call.
func (Piper) MaxChunk() int { return 1500 }

// Synthesize runs piper with --output_raw, which streams 16-bit mono PCM at
// the model's sample rate. Piper treats every input line as an utterance, so
// newlines are folded into spaces; the caller already split on sentences.
func (p Piper) Synthesize(ctx context.Context, v voices.Voice, text string, speed float64) (audio.PCM, error) {
	lengthScale := v.Params.LengthScale / (v.Params.Speed * speed)
	args := []string{
		"--quiet", "--output_raw",
		"--model", v.Model, "--config", v.Config,
		"--length_scale", strconv.FormatFloat(lengthScale, 'f', 3, 64),
	}
	if v.Params.Speaker > 0 {
		args = append(args, "--speaker", strconv.Itoa(v.Params.Speaker))
	}
	line := strings.Join(strings.Fields(text), " ") + "\n"
	out, err := p.Runner.Run(ctx, p.Bin, args, []byte(line))
	if err != nil {
		return audio.PCM{}, err
	}
	return audio.PCM{SampleRate: v.SampleRate, Channels: 1, Data: out[:len(out)-len(out)%2]}, nil
}
