package models

import (
	"math/rand"
	"strings"
	"testing"
)

// regexProseWords is the original implementation, kept as the reference the fast
// scanner is checked against (PERF-013).
func regexProseWords(s string) int {
	return len(strings.Fields(markupStripRe.ReplaceAllString(s, " ")))
}

// TestCountProseWordsMatchesRegex pins the hand-written scanner to the regex it
// replaced, over the tricky shapes plus randomly generated markup soup.
func TestCountProseWordsMatchesRegex(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"one two three",
		"<p>hello</p> world",
		"a<b>c</b>d",                    // markup splits a word into two
		"unterminated < tag stays text", // no closing '>' ⇒ ordinary text
		"{{ shortcode }} after",         // brace shortcode
		"{{ unterminated } brace",       // '}' not followed by '}' ⇒ text
		"{single} brace",                // single '{' is not a token
		"[shortcode attr=1] text",       // bracket shortcode
		"[/closing] text",               // closing bracket shortcode
		"[123] not a shortcode",         // must start with a letter
		"[unterminated text",            // no ']' ⇒ ordinary text
		"[a]",                           // minimal bracket token
		"<>", "<<>>", "{{}}", "[a]b[c]", //
		"emoji 🎉 counts as a word",          //
		"non breaking space",                // NBSP is unicode space
		"tabs\tand\nnewlines\r\nmixed",      //
		"<a href=\"x\">link</a>text",        //
		"nested <div>{{ var }}</div> stuff", //
		"trailing markup <p>",
		"<p>leading markup",
		strings.Repeat("word <b>x</b> ", 50),
	}
	for _, in := range cases {
		if got, want := countProseWords(in), regexProseWords(in); got != want {
			t.Errorf("countProseWords(%q) = %d, regex says %d", in, got, want)
		}
	}

	// Random markup soup: same alphabet the regex cares about, so the generator
	// produces plenty of terminated and unterminated tokens.
	rng := rand.New(rand.NewSource(1))
	atoms := []string{
		"a", "bb", " ", "  ", "\n", "\t", "<", ">", "{", "}", "[", "]", "/",
		"<p>", "</p>", "{{", "}}", "{{x}}", "[tag]", "[/tag]", "[1]", "é", "🎉",
	}
	for i := 0; i < 4000; i++ {
		var b strings.Builder
		for j := 0; j < rng.Intn(24); j++ {
			b.WriteString(atoms[rng.Intn(len(atoms))])
		}
		in := b.String()
		if got, want := countProseWords(in), regexProseWords(in); got != want {
			t.Fatalf("countProseWords(%q) = %d, regex says %d", in, got, want)
		}
	}
}

// TestComputeReadingStatsBoundaries covers the stats themselves: markup is excluded, the
// minute count rounds up, empty content is zero, and it stays idempotent.
func TestComputeReadingStatsBoundaries(t *testing.T) {
	p := &Page{Content: "<p>" + strings.Repeat("word ", 250) + "</p>"}
	p.ComputeReadingStats()
	if p.WordCount != 250 {
		t.Errorf("WordCount = %d, want 250 (markup excluded)", p.WordCount)
	}
	if p.ReadingTime != 2 { // ceil(250/200)
		t.Errorf("ReadingTime = %d, want 2", p.ReadingTime)
	}
	before := *p
	p.ComputeReadingStats()
	if p.WordCount != before.WordCount || p.ReadingTime != before.ReadingTime {
		t.Error("ComputeReadingStats must be idempotent")
	}

	empty := &Page{Content: "<p></p>{{ x }}"}
	empty.ComputeReadingStats()
	if empty.WordCount != 0 || empty.ReadingTime != 0 {
		t.Errorf("markup-only content = %d words / %d min, want 0/0", empty.WordCount, empty.ReadingTime)
	}

	one := &Page{Content: "word"}
	one.ComputeReadingStats()
	if one.ReadingTime != 1 {
		t.Errorf("any prose must read as at least 1 minute, got %d", one.ReadingTime)
	}
}

// benchDoc is a page-sized document with the markup mix a real post has.
var benchDoc = strings.Repeat(
	"<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit sed do.</p>\n"+
		"{{ shortcode arg=1 }} [note type=info] more prose here and there.\n", 60)

func BenchmarkCountProseWords(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		_ = countProseWords(benchDoc)
	}
}

func BenchmarkCountProseWordsRegex(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		_ = regexProseWords(benchDoc)
	}
}
