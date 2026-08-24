package images

// Fuzz targets for the two parsers here that read bytes ssg did not write.
//
// Both run over image files, and a migrated site's media comes from a server
// the operator does not control — so "malformed input" is the normal case, not
// a hypothetical. Neither parser may panic, whatever it is handed: a panic in a
// build is a build that stops, and the file that stopped it is the one file the
// operator cannot easily remove from someone else's export.

import (
	"bytes"
	"testing"
)

// FuzzOrientationFromTIFF drives the EXIF orientation reader with arbitrary
// bytes. It does its own bounds arithmetic over offsets read from the file —
// an IFD offset, an entry count, twelve-byte strides — which is the shape that
// goes wrong when the numbers are hostile rather than merely wrong.
func FuzzOrientationFromTIFF(f *testing.F) {
	// Both byte orders, a real orientation tag, and the truncations around it.
	f.Add([]byte("II*\x00\x08\x00\x00\x00"))
	f.Add([]byte("MM\x00*\x00\x00\x00\x08"))
	f.Add([]byte("II*\x00\x08\x00\x00\x00\x01\x00\x12\x01\x03\x00\x01\x00\x00\x00\x06\x00\x00\x00"))
	f.Add([]byte("II*\x00\xff\xff\xff\xff"))
	f.Add([]byte("MM"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		got := orientationFromTIFF(data)
		// The contract is an EXIF orientation, and callers index transform
		// tables with it. Anything outside 1..8 is a caller's panic later.
		if got < 1 || got > 8 {
			t.Fatalf("orientation = %d for %q, want 1..8", got, data)
		}
	})
}

// FuzzStripJPEG drives the metadata stripper. It walks marker segments using
// lengths the file itself declares, so a declared length that runs past the end
// — or wraps — is exactly what it has to survive.
func FuzzStripJPEG(f *testing.F) {
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xD9})                                     // shortest valid-ish JPEG
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0xFF, 0xD9})             // empty APP0
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 0x00, 0xFF, 0xD9})       // APP1 claiming more than exists
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xDA, 0x00, 0x02, 0x01, 0x02, 0xFF, 0xD9}) // SOS then entropy data
	f.Add([]byte("not a jpeg"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := stripJPEG(data)
		if err != nil {
			return // refusing malformed input is the correct outcome
		}
		// Stripping removes segments and adds none, so the result cannot grow.
		// A stripper that returns more than it was given has miscounted a
		// length, which is the bug class this target exists for.
		if len(out) > len(data) {
			t.Fatalf("stripping grew the file: %d → %d bytes", len(data), len(out))
		}
		// Whatever it returns still has to be a JPEG: the SOI it copied first.
		if !bytes.HasPrefix(out, []byte{0xFF, 0xD8}) {
			t.Fatalf("output is not a JPEG: % x", out[:min(len(out), 8)])
		}
	})
}
