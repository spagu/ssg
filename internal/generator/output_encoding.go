// Output encoding and content hygiene (GO-086/GO-087).
//
// cleanSpecialChars normalises the "smart" Western punctuation that AI tools
// routinely emit (curly quotes, en/em dashes, ellipsis, non-breaking and
// zero-width spaces) into plain ASCII. It targets a fixed allowlist only, so
// CJK text and every other script (and CJK's own full-width punctuation) pass
// through untouched.
//
// encodeText re-encodes finished UTF-8 output as UTF-16 (LE or BE, with a BOM)
// when a page's section asks for it; UTF-8 is a passthrough. Every encoding is
// Unicode, so all scripts round-trip losslessly.
package generator

import (
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/models"
	"golang.org/x/text/encoding/unicode"
)

const (
	encodingUTF8    = "utf-8"
	encodingUTF16LE = "utf-16le"
	encodingUTF16BE = "utf-16be"
)

// specialCharMap lists the AI/smart Unicode punctuation this filter rewrites,
// keyed by code point so the source stays pure ASCII (several of these are
// invisible or a byte-order mark that a literal would make illegal in Go
// source). Only these code points are ever touched.
var specialCharMap = []struct {
	from rune
	to   string
}{
	{0x2018, "'"},   // left single quote
	{0x2019, "'"},   // right single quote / apostrophe
	{0x201A, "'"},   // single low-9 quote
	{0x201B, "'"},   // single high-reversed-9 quote
	{0x201C, "\""},  // left double quote
	{0x201D, "\""},  // right double quote
	{0x201E, "\""},  // double low-9 quote
	{0x2033, "\""},  // double prime
	{0x2032, "'"},   // prime
	{0x2013, "-"},   // en dash
	{0x2014, "--"},  // em dash
	{0x2015, "--"},  // horizontal bar
	{0x2026, "..."}, // horizontal ellipsis
	{0x00A0, " "},   // non-breaking space
	{0x202F, " "},   // narrow no-break space
	{0x2009, " "},   // thin space
	{0x200B, ""},    // zero-width space
	{0x200C, ""},    // zero-width non-joiner
	{0x200D, ""},    // zero-width joiner
	{0xFEFF, ""},    // zero-width no-break space / stray BOM
	{0x2212, "-"},   // minus sign
	{0x00AB, "\""},  // left guillemet
	{0x00BB, "\""},  // right guillemet
}

// specialCharReplacer is built once from specialCharMap.
var specialCharReplacer = buildSpecialCharReplacer()

func buildSpecialCharReplacer() *strings.Replacer {
	args := make([]string, 0, len(specialCharMap)*2)
	for _, p := range specialCharMap {
		args = append(args, string(p.from), p.to)
	}
	return strings.NewReplacer(args...)
}

// cleanSpecialChars applies the AI-punctuation normalisation when enabled.
func (g *Generator) cleanSpecialChars(s string) string {
	if !g.config.CleanSpecialChars {
		return s
	}
	return specialCharReplacer.Replace(s)
}

// normalizeEncoding canonicalises an encoding name; unknown values fall back to
// UTF-8 so a typo never corrupts output.
func normalizeEncoding(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", encodingUTF8, "utf8":
		return encodingUTF8
	case encodingUTF16LE, "utf16le", "utf-16", "utf16":
		return encodingUTF16LE
	case encodingUTF16BE, "utf16be":
		return encodingUTF16BE
	default:
		return encodingUTF8
	}
}

// encodingFor resolves a page's output encoding: a section override (longest
// matching prefix, same rule as schema_defaults) wins over the global setting.
// A nil page (listing pages) uses the global encoding.
func (g *Generator) encodingFor(page *models.Page) string {
	enc := normalizeEncoding(g.config.OutputEncoding)
	if page == nil || len(g.config.OutputEncodingSections) == 0 {
		return enc
	}
	if strings.Trim(page.GetURL(), "/") == "" {
		if v, ok := g.config.OutputEncodingSections[homeSectionKey]; ok {
			return normalizeEncoding(v)
		}
	}
	section := g.contentSection(*page)
	if best, _ := sectionValue(g.config.OutputEncodingSections, section); best != "" {
		return normalizeEncoding(best)
	}
	return enc
}

// encodeText re-encodes UTF-8 text to the target encoding. UTF-16 output carries
// a byte-order mark; UTF-8 is returned unchanged.
func encodeText(s, encoding string) []byte {
	switch encoding {
	case encodingUTF16LE:
		return utf16Bytes(s, unicode.LittleEndian)
	case encodingUTF16BE:
		return utf16Bytes(s, unicode.BigEndian)
	default:
		return []byte(s)
	}
}

// utf16Bytes encodes s as UTF-16 with a BOM in the given byte order.
func utf16Bytes(s string, order unicode.Endianness) []byte {
	enc := unicode.UTF16(order, unicode.UseBOM).NewEncoder()
	out, err := enc.String(s)
	if err != nil {
		return []byte(s) // never corrupt output on an encode error
	}
	return []byte(out)
}

// metaCharsetRe matches an HTML meta charset declaration to rewrite for UTF-16.
var metaCharsetRe = regexp.MustCompile(`(?i)<meta\s+charset=["']?[a-z0-9-]+["']?\s*/?>`)

// setHTMLCharset rewrites the <meta charset> to match a UTF-16 encoding so the
// declaration agrees with the BOM. UTF-8 output is left untouched.
func setHTMLCharset(html, encoding string) string {
	if encoding == encodingUTF8 {
		return html
	}
	if metaCharsetRe.MatchString(html) {
		return metaCharsetRe.ReplaceAllString(html, `<meta charset="utf-16">`)
	}
	return html
}
