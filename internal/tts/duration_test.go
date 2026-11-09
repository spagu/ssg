package tts

import (
	"bytes"
	"testing"
	"time"
)

// frame is one Layer III frame: header plus zero payload.
func frame(t *testing.T, h []byte) []byte {
	t.Helper()
	n, _, _ := mp3Frame(h)
	if n == 0 {
		t.Fatalf("not a frame header: % x", h)
	}
	return append(append([]byte{}, h...), make([]byte, n-4)...)
}

// TestDuration: MPEG-1 at 128 kbit/s 44.1 kHz, MPEG-2 and 2.5, with ID3 tags
// and junk around the frames.
func TestDuration(t *testing.T) {
	v1 := frame(t, []byte{0xFF, 0xFB, 0x90, 0x00})  // MPEG-1 L3 128k 44.1k
	v1p := frame(t, []byte{0xFF, 0xFB, 0x92, 0x00}) // padded
	v2 := frame(t, []byte{0xFF, 0xF3, 0x84, 0x00})  // MPEG-2 L3 64k 24k
	v25 := frame(t, []byte{0xFF, 0xE3, 0x88, 0x00}) // MPEG-2.5 L3 64k 8k
	id3 := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 2, 0, 0}
	data := append(append([]byte{}, id3...), bytes.Repeat(v1, 38)...)
	data = append(data, v1p...)
	data = append(data, 0x00, 0x13)
	data = append(data, v2...)
	data = append(data, v25...)
	want := 39*1152.0/44100 + 576.0/24000 + 576.0/8000
	got := Duration(data).Seconds()
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("Duration = %.4f s, want %.4f s", got, want)
	}
	if Duration(nil) != 0 || Duration([]byte("not audio at all")) != 0 {
		t.Error("no frames, no time")
	}
	if d := Duration(bytes.Repeat(v1, 10)); d.Round(time.Millisecond) != 261*time.Millisecond {
		t.Errorf("ten frames = %v", d)
	}
}

// TestMP3FrameRejects: reserved values are not headers.
func TestMP3FrameRejects(t *testing.T) {
	for _, h := range [][]byte{
		{0x00, 0xFB, 0x90, 0x00}, // no sync
		{0xFF, 0xEB, 0x90, 0x00}, // reserved version
		{0xFF, 0xFD, 0x90, 0x00}, // layer II
		{0xFF, 0xFB, 0x0C, 0x00}, // reserved rate
		{0xFF, 0xFB, 0x00, 0x00}, // free bit rate
		{0xFF, 0xFB, 0xF0, 0x00}, // bad bit rate
	} {
		if n, _, _ := mp3Frame(h); n != 0 {
			t.Errorf("% x read as a frame", h)
		}
	}
}
