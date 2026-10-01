package engine

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

type fakeRunner struct {
	out  []byte
	err  error
	bin  string
	args []string
	in   string
}

func (f *fakeRunner) Run(_ context.Context, bin string, args []string, stdin []byte) ([]byte, error) {
	f.bin, f.args, f.in = bin, args, string(stdin)
	return f.out, f.err
}

func wav(t *testing.T) []byte {
	b, err := audio.PCM{SampleRate: 22050, Channels: 1, Data: make([]byte, 10)}.WAV()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEspeakSynthesize(t *testing.T) {
	r := &fakeRunner{out: wav(t)}
	e := Espeak{Bin: "espeak-ng", Runner: r}
	v := voices.Voice{EspeakVoice: "pl", Params: voices.Params{Speed: 1, Pitch: 60}}
	p, err := e.Synthesize(context.Background(), v, "Cześć", 2)
	if err != nil || len(p.Data) != 10 {
		t.Fatal(err)
	}
	want := []string{"--stdin", "--stdout", "-b", "1", "-v", "pl", "-s", "350", "-p", "60"}
	if !slices.Equal(r.args, want) || r.in != "Cześć" {
		t.Fatalf("%v %q", r.args, r.in)
	}
	_, _ = e.Synthesize(context.Background(), voices.Voice{EspeakVoice: "en", Params: voices.Params{Speed: 1}}, "x", 0.1)
	if r.args[7] != "80" || len(r.args) != 8 {
		t.Fatalf("clamp/pitch: %v", r.args)
	}
	r.err = errors.New("boom")
	if _, err := e.Synthesize(context.Background(), v, "x", 1); err == nil {
		t.Fatal("want error")
	}
	if e.MaxChunk() <= 0 || (Espeak{Bin: "no-such-espeak"}).Available() {
		t.Fatal("MaxChunk/Available")
	}
}

const voiceList = `Pty Language       Age/Gender VoiceName          File                 Other Languages
 5  af              --/M      Afrikaans          gmw/af
 2  en-gb           --/M      English_(Great_Britain) gmw/en               (en 2)
 5  en-gb-x-rp      --/F      English_(RP) gmw/en-GB-x-rp       (en-gb 4)(en 5)
 5  x               --/M      Broken             f
 5  zz-              --       Bad                f
`

func TestListVoices(t *testing.T) {
	r := &fakeRunner{out: []byte(voiceList)}
	list, err := Espeak{Bin: "espeak-ng", Runner: r}.ListVoices(context.Background())
	if err != nil || len(list) != 3 {
		t.Fatalf("%v %d", err, len(list))
	}
	gb, rp := list[1], list[2]
	if gb.ID != "espeak:en-gb" || gb.Name != "English (Great Britain)" || gb.Gender != "male" || gb.Aliases["en"] != 2 {
		t.Fatalf("%+v", gb)
	}
	if rp.Gender != "female" || rp.Aliases["en-gb"] != 4 || rp.Aliases["en"] != 5 {
		t.Fatalf("%+v", rp)
	}
	if gender("--") != "" || len(aliases("(en x)(bad)")) != 0 {
		t.Fatal("gender/aliases edge cases")
	}
	r.err = errors.New("boom")
	if _, err := (Espeak{Runner: r}).ListVoices(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestPiperSynthesize(t *testing.T) {
	r := &fakeRunner{out: []byte{1, 2, 3, 4, 5}}
	p := Piper{Bin: "piper", Runner: r}
	v := voices.Voice{Model: "/v/m.onnx", Config: "/v/m.onnx.json", SampleRate: 16000,
		Params: voices.Params{Speed: 1, LengthScale: 1, Speaker: 2}}
	pcm, err := p.Synthesize(context.Background(), v, "a\nb  c", 2)
	if err != nil || len(pcm.Data) != 4 || pcm.SampleRate != 16000 {
		t.Fatal(err, pcm)
	}
	if r.in != "a b c\n" || !slices.Contains(r.args, "0.500") || r.args[len(r.args)-1] != "2" {
		t.Fatalf("%q %v", r.in, r.args)
	}
	r.err = errors.New("boom")
	if _, err := p.Synthesize(context.Background(), v, "x", 1); err == nil {
		t.Fatal("want error")
	}
	if p.MaxChunk() <= 0 || (Piper{Bin: "no-such-piper"}).Available() {
		t.Fatal("MaxChunk/Available")
	}
}

func TestSplit(t *testing.T) {
	got := Split("One. Two! Three?\nFour; five… six", 100)
	if len(got) != 1 || got[0] != "One. Two! Three? Four; five… six" {
		t.Fatalf("%q", got)
	}
	got = Split("Alpha beta. Gamma delta. Epsilon.", 12)
	if !slices.Equal(got, []string{"Alpha beta.", "Gamma delta.", "Epsilon."}) {
		t.Fatalf("%q", got)
	}
	got = Split("aaaa bbbb cccc", 9)
	if !slices.Equal(got, []string{"aaaa bbbb", "cccc"}) {
		t.Fatalf("%q", got)
	}
	got = Split(strings.Repeat("ż", 25), 10)
	if len(got) != 3 || utf8.RuneCountInString(got[0]) != 10 {
		t.Fatalf("%q", got)
	}
	if len(Split("   \n ", 10)) != 0 || !slices.Equal(Split("v1.2 ok", 50), []string{"v1.2 ok"}) {
		t.Fatal("edge cases")
	}
}
