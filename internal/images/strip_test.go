package images

// Removing metadata from published images (#176). The fixtures are built here
// rather than checked in: a JPEG is a sequence of marker segments, so one can
// be assembled exactly, and a test that states the bytes it is asserting on is
// the one worth having.

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// segment builds one JPEG marker segment with the given payload.
func segment(marker byte, payload []byte) []byte {
	out := []byte{markerPrefix, marker, 0, 0}
	binary.BigEndian.PutUint16(out[2:4], uint16(len(payload)+2))
	return append(out, payload...)
}

// jpegWith assembles a minimal JPEG carrying the given segments between SOI and
// the scan.
func jpegWith(segments ...[]byte) []byte {
	out := []byte{markerPrefix, markerSOI}
	for _, s := range segments {
		out = append(out, s...)
	}
	// A scan and its (fake) entropy-coded data, then EOI.
	out = append(out, segment(markerSOS, []byte{0x01, 0x01})...)
	out = append(out, 0x11, 0x22, 0x33)
	return append(out, markerPrefix, markerEOI)
}

// TestGPSAndCameraDetailsAreRemoved: the reason this exists. A photo straight
// from a phone carries where it was taken and which device took it.
func TestGPSAndCameraDetailsAreRemoved(t *testing.T) {
	exif := append([]byte("Exif\x00\x00"), []byte("GPSLatitude 51.1079 SerialNumber 12345")...)
	iptc := append([]byte("Photoshop 3.0\x00"), []byte("author's name")...)
	comment := []byte("exported by a phone")

	src := jpegWith(
		segment(markerAPP0, []byte("JFIF\x00")),
		segment(markerAPP1, exif),
		segment(markerAPP13, iptc),
		segment(markerCOM, comment),
	)
	out := StripJPEGMetadata(src)

	for _, secret := range []string{"GPSLatitude", "SerialNumber", "author's name", "exported by a phone"} {
		if bytes.Contains(out, []byte(secret)) {
			t.Errorf("%q survived into the published image", secret)
		}
	}
	if !bytes.Contains(out, []byte("JFIF")) {
		t.Error("JFIF carries the density a viewer needs and must survive")
	}
	// Still a JPEG, and still carrying its image data.
	if !bytes.HasPrefix(out, []byte{markerPrefix, markerSOI}) {
		t.Error("the result must still start with SOI")
	}
	if !bytes.Contains(out, []byte{0x11, 0x22, 0x33}) {
		t.Error("the entropy-coded data must be untouched")
	}
	if !bytes.HasSuffix(out, []byte{markerPrefix, markerEOI}) {
		t.Error("the result must still end with EOI")
	}
}

// TestColourProfileSurvives: dropping an ICC profile shifts every colour on the
// page, which is a worse defect than the one being fixed.
func TestColourProfileSurvives(t *testing.T) {
	icc := append(append([]byte{}, iccSignature...), []byte{0x01, 0x02, 0x03}...)
	other := append([]byte("FPXR\x00"), 0x09)

	src := jpegWith(segment(markerAPP2, icc), segment(markerAPP2, other))
	out := StripJPEGMetadata(src)

	if !bytes.Contains(out, iccSignature) {
		t.Error("an ICC profile must survive")
	}
	if bytes.Contains(out, []byte("FPXR")) {
		t.Error("an APP2 segment that is not a colour profile must go")
	}
}

// TestACleanImageIsByteIdentical: a file with nothing to remove must come back
// exactly as it arrived, or every already-clean site's output changes.
func TestACleanImageIsByteIdentical(t *testing.T) {
	src := jpegWith(segment(markerAPP0, []byte("JFIF\x00")))
	if out := StripJPEGMetadata(src); !bytes.Equal(out, src) {
		t.Errorf("a clean image changed:\n%x\n%x", src, out)
	}
	if HasJPEGMetadata(src) {
		t.Error("a clean image must not be reported as carrying metadata")
	}
}

// TestAnythingNotUnderstoodIsPublishedUnchanged: a corrupted image is worse
// than one carrying a location, so refusing to guess is the rule.
func TestAnythingNotUnderstoodIsPublishedUnchanged(t *testing.T) {
	cases := map[string][]byte{
		"not a JPEG":        []byte("\x89PNG\r\n\x1a\n and more"),
		"empty":             {},
		"truncated":         {markerPrefix, markerSOI},
		"bad marker":        {markerPrefix, markerSOI, 0x00, 0x01, 0x02},
		"overrunning size":  {markerPrefix, markerSOI, markerPrefix, markerAPP1, 0xFF, 0xFF, 0x00},
		"segment too short": {markerPrefix, markerSOI, markerPrefix, markerAPP1, 0x00, 0x01},
	}
	for name, src := range cases {
		out := StripJPEGMetadata(src)
		if !bytes.Equal(out, src) {
			t.Errorf("%s: must be published unchanged, got %x", name, out)
		}
		if HasJPEGMetadata(src) {
			t.Errorf("%s: an unparseable file carries nothing this can report", name)
		}
	}
}

// TestMarkerPaddingIsTolerated: a run of 0xFF before a marker is legal, and a
// parser that trips on it would refuse to clean an ordinary file.
func TestMarkerPaddingIsTolerated(t *testing.T) {
	src := []byte{markerPrefix, markerSOI, markerPrefix, markerPrefix}
	src = append(src, segment(markerAPP1, []byte("Exif\x00\x00secret"))...)
	src = append(src, segment(markerSOS, []byte{0x01})...)
	src = append(src, markerPrefix, markerEOI)

	out := StripJPEGMetadata(src)
	if bytes.Contains(out, []byte("secret")) {
		t.Errorf("padding must not stop the strip: %x", out)
	}
}

// TestHasJPEGMetadataMatchesWhatStripWouldDo, so a caller can count without
// rewriting a file.
func TestHasJPEGMetadataMatchesWhatStripWouldDo(t *testing.T) {
	dirty := jpegWith(segment(markerAPP1, []byte("Exif\x00\x00x")))
	clean := jpegWith(segment(markerAPP0, []byte("JFIF\x00")))

	if !HasJPEGMetadata(dirty) {
		t.Error("an image with EXIF must be reported")
	}
	if HasJPEGMetadata(clean) {
		t.Error("an image without it must not")
	}
}

// TestPayloadOnASegmentTooShortToHaveOne: the ICC check reads a segment's
// bytes, and a truncated one must answer "no profile" rather than panic.
func TestPayloadEdges(t *testing.T) {
	if got := payload([]byte{0xFF, markerAPP2}); got != nil {
		t.Errorf("a segment with no length has no payload: %v", got)
	}
	if got := payload([]byte{0xFF, markerAPP2, 0x00, 0x02}); len(got) != 0 {
		t.Errorf("an empty payload = %v", got)
	}
	full := segment(markerAPP2, []byte("abc"))
	if got := payload(full); string(got) != "abc" {
		t.Errorf("payload = %q", got)
	}
}

// TestMarkersWithoutSegmentsArePassedThrough: the standalone markers (RSTn) and
// a segment with no data must survive a strip untouched.
func TestStandaloneMarkersSurvive(t *testing.T) {
	src := []byte{markerPrefix, markerSOI}
	src = append(src, segment(0xDB, []byte{0x01, 0x02})...) // quantisation table
	src = append(src, segment(0xC4, []byte{0x03})...)       // Huffman table
	src = append(src, segment(markerSOS, []byte{0x01})...)
	src = append(src, 0xAA, 0xBB, markerPrefix, markerEOI)

	out := StripJPEGMetadata(src)
	if !bytes.Equal(out, src) {
		t.Errorf("a file of nothing but structural segments must be unchanged:\n%x\n%x", src, out)
	}
}

// TestEOIBeforeAnySegment is a degenerate but legal file.
func TestEOIImmediately(t *testing.T) {
	src := []byte{markerPrefix, markerSOI, markerPrefix, markerEOI}
	if out := StripJPEGMetadata(src); !bytes.Equal(out, src) {
		t.Errorf("out = %x", out)
	}
}
