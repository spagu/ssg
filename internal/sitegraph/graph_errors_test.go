package sitegraph

import (
	"testing"
	"time"
)

// unencodableTime is the one value the model can legally hold that JSON
// refuses to render: time.Time rejects a year outside [0,9999]. It is how the
// tests below reach the encoder's failure paths without faking a writer.
func unencodableTime() time.Time {
	return time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestEncodeReportsAGraphItCannotRender: a graph the encoder chokes on comes
// back as an error and NO bytes, so a caller can never mistake the half-filled
// buffer for a finished artifact.
func TestEncodeReportsAGraphItCannotRender(t *testing.T) {
	g := sample(1)
	g.Build.Time = unencodableTime()
	data, err := Encode(g)
	if err == nil {
		t.Fatalf("a year past 9999 encoded anyway:\n%s", data)
	}
	if data != nil {
		t.Errorf("bytes returned alongside the error:\n%s", data)
	}
}

// TestHashReportsAGraphItCannotRender: Hash drops the build stamp before it
// encodes, so an unrenderable stamp cannot fail it — and when the CONTENT is
// what cannot be encoded, it returns an empty hash rather than the hash of a
// partial encoding.
func TestHashReportsAGraphItCannotRender(t *testing.T) {
	g := sample(1)
	g.Build.Time = unencodableTime()
	h, err := Hash(g)
	if err != nil {
		t.Fatalf("Hash must not look at the build stamp: %v", err)
	}
	if h == "" {
		t.Error("empty hash for a graph whose content is fine")
	}

	bad := unencodableTime()
	g = sample(1)
	g.Pages[0].Date = &bad
	h, err = Hash(g)
	if err == nil {
		t.Fatal("an unencodable page date hashed anyway")
	}
	if h != "" {
		t.Errorf("Hash = %q with an error; want the empty string", h)
	}
}

// TestStampKeepsTheOldHashWhenItCannotCompute: a failed Stamp reports the
// error and leaves Build.Hash exactly as it was, so a graph never ships a
// stamp that does not describe it.
func TestStampKeepsTheOldHashWhenItCannotCompute(t *testing.T) {
	bad := unencodableTime()
	g := sample(1)
	g.Build.Hash = "previous"
	g.Pages[0].Date = &bad
	if err := g.Stamp(); err == nil {
		t.Fatal("Stamp accepted a graph it could not hash")
	}
	if g.Build.Hash != "previous" {
		t.Errorf("Build.Hash = %q after a failed Stamp; want it untouched", g.Build.Hash)
	}
}
