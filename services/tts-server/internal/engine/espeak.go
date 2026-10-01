package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/execx"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// espeak-ng speaking rate bounds and default, in words per minute.
const (
	espeakBaseWPM = 175
	espeakMinWPM  = 80
	espeakMaxWPM  = 450
)

// Espeak drives the espeak-ng command line.
type Espeak struct {
	Bin    string
	Runner execx.Runner
}

// Available reports whether espeak-ng is installed.
func (e Espeak) Available() bool { return execx.Available(e.Bin) }

// MaxChunk keeps each espeak-ng call short enough to stay responsive.
func (Espeak) MaxChunk() int { return 4000 }

// Synthesize runs espeak-ng with the text on stdin (-b 1: UTF-8) and parses
// the WAV it writes to stdout.
func (e Espeak) Synthesize(ctx context.Context, v voices.Voice, text string, speed float64) (audio.PCM, error) {
	wpm := int(espeakBaseWPM * v.Params.Speed * speed)
	wpm = min(max(wpm, espeakMinWPM), espeakMaxWPM)
	args := []string{"--stdin", "--stdout", "-b", "1", "-v", v.EspeakVoice, "-s", strconv.Itoa(wpm)}
	if v.Params.Pitch > 0 {
		args = append(args, "-p", strconv.Itoa(v.Params.Pitch))
	}
	out, err := e.Runner.Run(ctx, e.Bin, args, []byte(text))
	if err != nil {
		return audio.PCM{}, err
	}
	return audio.ParseWAV(out)
}

// ListVoices asks espeak-ng for its languages and exposes each one as the
// voice "espeak:<lang>".
func (e Espeak) ListVoices(ctx context.Context) ([]voices.Voice, error) {
	out, err := e.Runner.Run(ctx, e.Bin, []string{"--voices"}, nil)
	if err != nil {
		return nil, fmt.Errorf("list espeak voices: %w", err)
	}
	return ParseEspeakVoices(out), nil
}

// ParseEspeakVoices reads `espeak-ng --voices` output:
//
//	Pty Language  Age/Gender VoiceName  File    Other Languages
//	 2  en-gb     --/M       English_.. gmw/en  (en 2)
func ParseEspeakVoices(out []byte) []voices.Voice {
	var list []voices.Voice
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || f[0] == "Pty" || !voices.ValidLang(f[1]) {
			continue
		}
		list = append(list, voices.Voice{
			ID: "espeak:" + f[1], Engine: voices.EngineEspeak,
			Lang: voices.NormalizeLang(f[1]), Name: strings.ReplaceAll(f[3], "_", " "),
			Gender: gender(f[2]), Quality: "low", SampleRate: 22050,
			Params:      voices.Params{Speed: 1},
			EspeakVoice: f[1], Revision: "espeak",
			Aliases: aliases(strings.Join(f[5:], " ")),
		})
	}
	return list
}

// gender maps espeak's "--/M" column to a word.
func gender(ageGender string) string {
	_, g, _ := strings.Cut(ageGender, "/")
	switch g {
	case "M":
		return "male"
	case "F":
		return "female"
	}
	return ""
}

// aliases parses "(en-gb 3)(en 5)" into {"en-gb": 3, "en": 5}.
func aliases(s string) map[string]int {
	out := map[string]int{}
	for part := range strings.SplitSeq(s, "(") {
		f := strings.Fields(strings.TrimSuffix(strings.TrimSpace(part), ")"))
		if len(f) != 2 {
			continue
		}
		if p, err := strconv.Atoi(f[1]); err == nil {
			out[voices.NormalizeLang(f[0])] = p
		}
	}
	return out
}
