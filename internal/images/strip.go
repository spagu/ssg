package images

// Removing metadata from published images (#176).
//
// A photo straight from a phone carries GPS coordinates, the camera's serial
// number and often the owner's name. Derivatives lose all of it for free — the
// standard library's encoders write only pixels — but ORIGINALS are copied byte
// for byte, and a migration copies a whole media library across. So a site can
// publish the author's home address without anyone choosing to.
//
// JPEG is a sequence of marker segments, which makes this exact rather than a
// guess: the segments carrying metadata are dropped and everything else is
// passed through untouched. Two are deliberately kept:
//
//   - APP0 (JFIF), which carries the density a viewer needs;
//   - APP2 when it holds an ICC colour profile — dropping that shifts every
//     colour on the page, which is a worse defect than the one being fixed.
//
// EXIF orientation is a special case and the reason this is not simply "drop
// APP1": a stripped orientation tag rotates photos that were correct. The
// caller normalises orientation into the pixels first, so by the time the tag
// goes, it has already been honoured.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// JPEG markers. A segment is 0xFF, the marker, then a two-byte length that
// includes itself.
const (
	markerPrefix = 0xFF
	markerSOI    = 0xD8 // start of image
	markerSOS    = 0xDA // start of scan: entropy-coded data follows, not segments
	markerEOI    = 0xD9
	markerAPP0   = 0xE0 // JFIF — kept
	markerAPP1   = 0xE1 // EXIF / XMP — dropped
	markerAPP2   = 0xE2 // ICC profile — kept; anything else here is dropped
	markerAPP13  = 0xED // Photoshop IRB, where IPTC lives — dropped
	markerCOM    = 0xFE // comment — dropped
)

// iccSignature identifies an APP2 segment carrying a colour profile.
var iccSignature = []byte("ICC_PROFILE\x00")

// StripJPEGMetadata returns src with its metadata segments removed, or src
// unchanged when it is not a JPEG or cannot be parsed.
//
// Refusing to guess is the point: a file this does not fully understand is
// published exactly as it arrived, because a corrupted image is worse than one
// carrying a location.
func StripJPEGMetadata(src []byte) []byte {
	out, err := stripJPEG(src)
	if err != nil {
		return src
	}
	return out
}

// stripJPEG walks the marker segments, copying what belongs in a published
// image and dropping what describes where it was taken.
func stripJPEG(src []byte) ([]byte, error) {
	if len(src) < 4 || src[0] != markerPrefix || src[1] != markerSOI {
		return nil, fmt.Errorf("not a JPEG")
	}
	var out bytes.Buffer
	out.Grow(len(src))
	out.Write(src[:2]) // SOI

	for at := 2; ; {
		if at+1 >= len(src) {
			return nil, io.ErrUnexpectedEOF
		}
		if src[at] != markerPrefix {
			return nil, fmt.Errorf("expected a marker at %d", at)
		}
		marker := src[at+1]
		// Padding: a run of 0xFF bytes before a marker is legal.
		if marker == markerPrefix {
			at++
			continue
		}
		if marker == markerEOI {
			out.Write(src[at:])
			return out.Bytes(), nil
		}
		// From the scan onward the file is entropy-coded data, not segments —
		// it is copied wholesale.
		if marker == markerSOS {
			out.Write(src[at:])
			return out.Bytes(), nil
		}
		if at+4 > len(src) {
			return nil, io.ErrUnexpectedEOF
		}
		size := int(binary.BigEndian.Uint16(src[at+2 : at+4]))
		if size < 2 || at+2+size > len(src) {
			return nil, fmt.Errorf("segment length %d overruns the file", size)
		}
		segment := src[at : at+2+size]
		if keepSegment(marker, segment) {
			out.Write(segment)
		}
		at += 2 + size
	}
}

// keepSegment decides one segment's fate.
func keepSegment(marker byte, segment []byte) bool {
	switch marker {
	case markerAPP1, markerAPP13, markerCOM:
		// EXIF (with its GPS block), XMP, IPTC, and whatever an editor wrote
		// into the comment.
		return false
	case markerAPP2:
		// Only for its colour profile. Dropping that shifts every colour.
		return bytes.HasPrefix(payload(segment), iccSignature)
	default:
		// APP0/JFIF, the quantisation and Huffman tables, the frame header:
		// everything a decoder needs.
		return true
	}
}

// payload returns a segment's bytes after the marker and length.
func payload(segment []byte) []byte {
	if len(segment) < 4 {
		return nil
	}
	return segment[4:]
}

// HasJPEGMetadata reports whether src carries any segment this would remove,
// so a caller can count what it cleaned without re-encoding a file that was
// already clean.
func HasJPEGMetadata(src []byte) bool {
	stripped, err := stripJPEG(src)
	return err == nil && len(stripped) != len(src)
}
