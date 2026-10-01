package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// ErrNotWAV is returned for input that is not 16-bit PCM RIFF/WAVE.
var ErrNotWAV = errors.New("audio: not a 16-bit PCM WAV stream")

const wavHeaderSize = 44

// ParseWAV extracts PCM from a RIFF/WAVE stream. Streams written to a pipe
// (espeak-ng --stdout) carry placeholder chunk sizes, so a data chunk that
// claims more bytes than remain is truncated to what is actually there.
func ParseWAV(b []byte) (PCM, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return PCM{}, ErrNotWAV
	}
	var pcm PCM
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := b[off+8:]
		if size > len(body) {
			size = len(body)
		}
		switch id {
		case "fmt ":
			if err := parseFmt(body[:size], &pcm); err != nil {
				return PCM{}, err
			}
		case "data":
			if pcm.SampleRate == 0 {
				return PCM{}, fmt.Errorf("%w: data before fmt", ErrNotWAV)
			}
			frame := 2 * pcm.Channels
			pcm.Data = body[:size-size%frame]
			return pcm, nil
		}
		off += 8 + size + size%2
	}
	return PCM{}, fmt.Errorf("%w: no data chunk", ErrNotWAV)
}

func parseFmt(b []byte, pcm *PCM) error {
	if len(b) < 16 {
		return fmt.Errorf("%w: short fmt chunk", ErrNotWAV)
	}
	format := binary.LittleEndian.Uint16(b[0:2])
	bits := binary.LittleEndian.Uint16(b[14:16])
	if format != 1 || bits != 16 {
		return fmt.Errorf("%w: format %d, %d bits", ErrNotWAV, format, bits)
	}
	pcm.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
	pcm.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
	if pcm.Channels == 0 || pcm.SampleRate == 0 {
		return fmt.Errorf("%w: zero channels or sample rate", ErrNotWAV)
	}
	return nil
}

// WAV serialises p as a canonical 44-byte-header WAV file.
func (p PCM) WAV() ([]byte, error) {
	if !p.Valid() || len(p.Data) > math.MaxUint32-wavHeaderSize {
		return nil, fmt.Errorf("audio: invalid PCM (%d Hz, %d ch, %d bytes)", p.SampleRate, p.Channels, len(p.Data))
	}
	le := binary.LittleEndian
	out := make([]byte, wavHeaderSize, wavHeaderSize+len(p.Data))
	copy(out[0:], "RIFF")
	le.PutUint32(out[4:], uint32(36+len(p.Data))) // #nosec G115 -- len checked against MaxUint32 above
	copy(out[8:], "WAVEfmt ")
	le.PutUint32(out[16:], 16)
	le.PutUint16(out[20:], 1)
	le.PutUint16(out[22:], uint16(p.Channels))                // #nosec G115 -- Valid: <= maxChannels
	le.PutUint32(out[24:], uint32(p.SampleRate))              // #nosec G115 -- Valid: <= maxSampleRate
	le.PutUint32(out[28:], uint32(p.SampleRate*2*p.Channels)) // #nosec G115 -- Valid bounds both factors
	le.PutUint16(out[32:], uint16(2*p.Channels))              // #nosec G115 -- Valid bounds both factors
	le.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	le.PutUint32(out[40:], uint32(len(p.Data))) // #nosec G115 -- len checked against MaxUint32 above
	return append(out, p.Data...), nil
}
