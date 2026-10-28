package theme

// Archive entries that open or read badly fail the extraction instead of
// leaving a half-written file reported as success.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

// zipWith returns a one-entry archive's reader after patch has edited its bytes.
func zipWith(t *testing.T, method uint16, body string, patch func([]byte)) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: method})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(body))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	patch(data)
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return r.File[0]
}

// TestExtractZipEntryUnsupportedMethod: an entry compressed with a method Go
// cannot read is an error.
func TestExtractZipEntryUnsupportedMethod(t *testing.T) {
	f := zipWith(t, zip.Store, "hello", func(b []byte) {
		// The method field sits at offset 8 of the local header and 10 of the
		// central directory header.
		binary.LittleEndian.PutUint16(b[8:], 99)
		if i := bytes.Index(b, []byte("PK\x01\x02")); i >= 0 {
			binary.LittleEndian.PutUint16(b[i+10:], 99)
		}
	})
	if _, err := extractZipEntry(f, filepath.Join(t.TempDir(), "a.txt"), 1<<20); err == nil {
		t.Error("want an error for an unsupported method")
	}
}

// TestExtractZipEntryCorruptData: compressed bytes that do not inflate fail.
func TestExtractZipEntryCorruptData(t *testing.T) {
	f := zipWith(t, zip.Deflate, strings.Repeat("ssg ", 200), func(b []byte) {
		// Overwrite the start of the compressed stream, just past the local
		// header (30 bytes) and the 5-byte name.
		for i := 35; i < 45; i++ {
			b[i] = 0xff
		}
	})
	if _, err := extractZipEntry(f, filepath.Join(t.TempDir(), "a.txt"), 1<<20); err == nil {
		t.Error("want an error for corrupt data")
	}
}
