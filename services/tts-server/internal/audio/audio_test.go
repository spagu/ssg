package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

type fakeRunner struct {
	out  []byte
	err  error
	args []string
	in   []byte
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string, stdin []byte) ([]byte, error) {
	f.args, f.in = args, stdin
	return f.out, f.err
}

func pcm(n int) PCM { return PCM{SampleRate: 22050, Channels: 1, Data: make([]byte, n)} }

func TestWAVRoundTrip(t *testing.T) {
	p := pcm(8)
	p.Data[0] = 7
	b, err := p.WAV()
	if err != nil || len(b) != 52 {
		t.Fatal(err, len(b))
	}
	got, err := ParseWAV(b)
	if err != nil || got.SampleRate != 22050 || got.Channels != 1 || len(got.Data) != 8 || got.Data[0] != 7 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestParseWAVStreamingHeaderAndPadding(t *testing.T) {
	b, _ := pcm(6).WAV()
	// Insert an odd-sized LIST chunk before data, like real encoders do.
	list := append([]byte("LIST"), 3, 0, 0, 0, 'a', 'b', 'c', 0)
	b = append(append(append([]byte{}, b[:36]...), list...), b[36:]...)
	binary.LittleEndian.PutUint32(b[len(b)-6-4:], 0x7ffff000) // placeholder data size
	got, err := ParseWAV(b)
	if err != nil || len(got.Data) != 6 {
		t.Fatalf("%v %d", err, len(got.Data))
	}
}

func TestParseWAVErrors(t *testing.T) {
	good, _ := pcm(4).WAV()
	mutate := func(f func(b []byte) []byte) []byte { return f(append([]byte{}, good...)) }
	cases := map[string][]byte{
		"short":     []byte("RIFF"),
		"not riff":  mutate(func(b []byte) []byte { b[0] = 'X'; return b }),
		"short fmt": mutate(func(b []byte) []byte { b[16] = 4; return b[:28] }),
		"float":     mutate(func(b []byte) []byte { b[20] = 3; return b }),
		"zero ch":   mutate(func(b []byte) []byte { b[22] = 0; return b }),
		"no data":   mutate(func(b []byte) []byte { return b[:36] }),
		"data first": mutate(func(b []byte) []byte {
			copy(b[12:16], "data")
			return b
		}),
	}
	for name, b := range cases {
		if _, err := ParseWAV(b); !errors.Is(err, ErrNotWAV) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestAppend(t *testing.T) {
	var empty PCM
	a, err := empty.Append(pcm(2))
	if err != nil || len(a.Data) != 2 {
		t.Fatal(err)
	}
	b, err := a.Append(pcm(4))
	if err != nil || len(b.Data) != 6 {
		t.Fatal(err)
	}
	if _, err := a.Append(PCM{SampleRate: 16000, Channels: 1}); !errors.Is(err, ErrFormatMismatch) {
		t.Fatal(err)
	}
}

func TestWAVInvalid(t *testing.T) {
	if _, err := (PCM{}).WAV(); err == nil {
		t.Fatal("want error")
	}
}

func TestFormat(t *testing.T) {
	for in, want := range map[string]Format{"": FormatMP3, "MP3": FormatMP3, " wav ": FormatWAV} {
		if got, err := ParseFormat(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseFormat("ogg"); err == nil {
		t.Error("want error")
	}
	if FormatWAV.ContentType() != "audio/wav" || FormatMP3.ContentType() != "audio/mpeg" {
		t.Error("content types")
	}
}

func TestEncoder(t *testing.T) {
	r := &fakeRunner{out: []byte("ID3mp3")}
	e := Encoder{LameBin: "lame", Runner: r}
	mp3, err := e.Encode(context.Background(), pcm(4), FormatMP3)
	if err != nil || string(mp3) != "ID3mp3" || string(r.in[:4]) != "RIFF" || r.args[len(r.args)-1] != "-" {
		t.Fatal(err, string(mp3))
	}
	wav, err := e.Encode(context.Background(), pcm(4), FormatWAV)
	if err != nil || string(wav[:4]) != "RIFF" {
		t.Fatal(err)
	}
	if _, err := e.Encode(context.Background(), PCM{}, FormatMP3); err == nil {
		t.Fatal("want invalid pcm error")
	}
	r.err = errors.New("boom")
	if _, err := e.Encode(context.Background(), pcm(4), FormatMP3); err == nil {
		t.Fatal("want runner error")
	}
	if (Encoder{LameBin: "definitely-missing-binary"}).Available() {
		t.Fatal("missing binary reported available")
	}
}
