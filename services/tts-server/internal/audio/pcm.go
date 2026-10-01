// Package audio holds the PCM representation shared by every engine, a WAV
// reader/writer, and the MP3 encoder.
package audio

import (
	"errors"
	"fmt"
)

// PCM is signed 16-bit little-endian interleaved audio.
type PCM struct {
	SampleRate int
	Channels   int
	Data       []byte
}

// ErrFormatMismatch is returned when concatenating audio of different shapes.
var ErrFormatMismatch = errors.New("audio: cannot join clips with different sample rate or channels")

// Upper bounds a PCM header may carry; they keep every WAV header field
// within its 16- or 32-bit slot.
const (
	maxSampleRate = 384000
	maxChannels   = 8
)

// Valid reports whether p describes playable audio.
func (p PCM) Valid() bool {
	return p.SampleRate > 0 && p.SampleRate <= maxSampleRate &&
		p.Channels > 0 && p.Channels <= maxChannels && len(p.Data)%(2*p.Channels) == 0
}

// Append returns p followed by next. An empty p adopts next's format.
func (p PCM) Append(next PCM) (PCM, error) {
	if p.SampleRate == 0 {
		return next, nil
	}
	if p.SampleRate != next.SampleRate || p.Channels != next.Channels {
		return PCM{}, fmt.Errorf("%w: %d/%d vs %d/%d", ErrFormatMismatch,
			p.SampleRate, p.Channels, next.SampleRate, next.Channels)
	}
	data := make([]byte, 0, len(p.Data)+len(next.Data))
	data = append(append(data, p.Data...), next.Data...)
	return PCM{SampleRate: p.SampleRate, Channels: p.Channels, Data: data}, nil
}
