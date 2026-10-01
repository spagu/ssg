package audio

import (
	"context"
	"fmt"
	"strings"

	"github.com/spagu/ssg/services/tts-server/internal/execx"
)

// Format is an output container the API can return.
type Format string

// Supported output formats.
const (
	FormatMP3 Format = "mp3"
	FormatWAV Format = "wav"
)

// ParseFormat maps a request value to a Format; empty means MP3.
func ParseFormat(s string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(s))) {
	case "", FormatMP3:
		return FormatMP3, nil
	case FormatWAV:
		return FormatWAV, nil
	}
	return "", fmt.Errorf("unsupported format %q (use mp3 or wav)", s)
}

// ContentType is the MIME type served for f.
func (f Format) ContentType() string {
	if f == FormatWAV {
		return "audio/wav"
	}
	return "audio/mpeg"
}

// mp3Bitrate is a constant bitrate (kbit/s) that keeps mono speech clear.
const mp3Bitrate = "64"

// Encoder turns PCM into the requested container, using lame for MP3.
type Encoder struct {
	LameBin string
	Runner  execx.Runner
}

// Encode returns pcm as WAV bytes, or pipes that WAV through lame for MP3.
func (e Encoder) Encode(ctx context.Context, pcm PCM, f Format) ([]byte, error) {
	wav, err := pcm.WAV()
	if err != nil || f == FormatWAV {
		return wav, err
	}
	args := []string{"--quiet", "-m", "m", "-b", mp3Bitrate, "-", "-"}
	mp3, err := e.Runner.Run(ctx, e.LameBin, args, wav)
	if err != nil {
		return nil, fmt.Errorf("mp3 encode: %w", err)
	}
	return mp3, nil
}

// Available reports whether the MP3 encoder binary is installed.
func (e Encoder) Available() bool { return execx.Available(e.LameBin) }
