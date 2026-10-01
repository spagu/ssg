package tts

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunk(t *testing.T) {
	if Chunk("  \n ", 10) != nil {
		t.Error("blank text gives no chunks")
	}
	if got := Chunk("a  b\nc", 0); len(got) != 1 || got[0] != "a b c" {
		t.Errorf("max 0 = %q", got)
	}
	text := "First sentence here. Second one is longer than that. Third."
	for _, c := range Chunk(text, 30) {
		if utf8.RuneCountInString(c) > 30 {
			t.Errorf("chunk over max: %q", c)
		}
	}
	if got := Chunk(text, 30); got[0] != "First sentence here." {
		t.Errorf("cut at the sentence end, got %q", got)
	}
	if got := Chunk("Zażółć gęślą jaźń kocha", 10); strings.Join(got, " ") != "Zażółć gęślą jaźń kocha" {
		t.Errorf("multibyte text must survive: %q", got)
	}
	if got := Chunk("abcdefghij", 4); strings.Join(got, "") != "abcdefghij" || got[0] != "abcd" {
		t.Errorf("a long word is cut mid-word: %q", got)
	}
}

func TestJoinStripsTags(t *testing.T) {
	id3v2 := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x02"), 'x', 'y')
	footer := append([]byte("ID3\x04\x00\x10\x00\x00\x00\x00"), bytes.Repeat([]byte{0}, 10)...)
	v1 := append([]byte("TAG"), bytes.Repeat([]byte{' '}, 125)...)
	got := Join(append([]byte("ID3a"), v1...), append(id3v2, []byte("B")...), append(footer, append([]byte("C"), v1...)...))
	want := append([]byte("ID3aBC"), v1...)
	if !bytes.Equal(got, want) {
		t.Errorf("Join = %q, want %q", got, want)
	}
	short := []byte("ID3\x04\x00\x00\x00\x00\x7f\x7f")
	if !bytes.Equal(stripID3v2(short), short) {
		t.Error("a tag claiming more than the data must be left alone")
	}
}

func TestFetchJingle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.mp3":
			_, _ = w.Write([]byte("JINGLE"))
		case "/big.mp3":
			_, _ = w.Write(bytes.Repeat([]byte{1}, maxJingleBytes+1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if data, err := FetchJingle(context.Background(), srv.URL+"/ok.mp3", 0); err != nil || string(data) != "JINGLE" {
		t.Errorf("ok = %q, %v", data, err)
	}
	for _, u := range []string{srv.URL + "/missing.mp3", srv.URL + "/big.mp3", "file:///etc/passwd", "ftp://x/y", "http://127.0.0.1:1/x.mp3"} {
		if _, err := FetchJingle(context.Background(), u, 0); err == nil {
			t.Errorf("FetchJingle(%q) must fail", u)
		}
	}
}
