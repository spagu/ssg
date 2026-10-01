package tts

import (
	"strings"
	"unicode/utf8"
)

// Chunk splits text into pieces of at most max characters, cutting after a
// sentence end where it can, at a space where it cannot, and mid-word only
// when a single word is longer than max. Whitespace is normalised; empty
// input gives no chunks.
func Chunk(text string, max int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	if max <= 0 {
		return []string{text}
	}
	var chunks []string
	for utf8.RuneCountInString(text) > max {
		cut := cutPoint(text, max)
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

// cutPoint is the byte offset to cut text at, within its first max runes.
func cutPoint(text string, max int) int {
	limit := byteOffset(text, max)
	head := text[:limit]
	for _, end := range []string{". ", "! ", "? ", "; ", ": ", ", "} {
		if i := strings.LastIndex(head, end); i > limit/2 {
			return i + len(end)
		}
	}
	if i := strings.LastIndex(head, " "); i > 0 {
		return i + 1
	}
	return limit
}

// byteOffset is the byte index of the n-th rune.
func byteOffset(s string, n int) int {
	i := 0
	for n > 0 && i < len(s) {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n--
	}
	return i
}

// Join concatenates MP3 parts into one stream. MP3 is a sequence of
// self-contained frames, so parts of the same encoding play back to back; only
// the tags in between must go — an ID3v2 header at the start of every part but
// the first, and an ID3v1 trailer at the end of every part but the last, would
// otherwise be read as audio and click.
func Join(parts ...[]byte) []byte {
	var out []byte
	for i, p := range parts {
		if i > 0 {
			p = stripID3v2(p)
		}
		if i < len(parts)-1 {
			p = stripID3v1(p)
		}
		out = append(out, p...)
	}
	return out
}

// stripID3v2 drops a leading ID3v2 tag: "ID3", version, flags, then a
// 28-bit synchsafe size, plus a 10-byte footer when flag 0x10 is set.
func stripID3v2(b []byte) []byte {
	if len(b) < 10 || string(b[:3]) != "ID3" {
		return b
	}
	size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
	total := 10 + size
	if b[5]&0x10 != 0 {
		total += 10
	}
	if total > len(b) {
		return b
	}
	return b[total:]
}

// stripID3v1 drops a trailing 128-byte ID3v1 tag.
func stripID3v1(b []byte) []byte {
	if len(b) >= 128 && string(b[len(b)-128:len(b)-125]) == "TAG" {
		return b[:len(b)-128]
	}
	return b
}
