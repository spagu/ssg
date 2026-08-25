package mcp

// Media the content manager can change (#214).

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// onePNG is the smallest valid PNG: a 1×1 transparent pixel.
var onePNG = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
	0, 0, 0, 0x0D, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0,
	0x1F, 0x15, 0xC4, 0x89,
	0, 0, 0, 0x0A, 'I', 'D', 'A', 'T', 0x78, 0x9C, 0x63, 0, 1, 0, 0, 5, 0, 1,
	0x0D, 0x0A, 0x2D, 0xB4,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xAE, 0x42, 0x60, 0x82,
}

func b64(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

// TestTheReportedRequestNowHasAnAnswer: "change the picture on the about page"
// used to end in the assistant saying it could not (#214).
func TestTheReportedRequestNowHasAnAnswer(t *testing.T) {
	s, root := newTestServer(t, nil)

	res := call(t, s, "media_upload", map[string]any{
		"path": "static/images/hero.png", "content_base64": b64(onePNG),
	})
	if res.IsError {
		t.Fatalf("upload failed: %s", text(res))
	}
	if got := readProjectFile(t, filepath.Join(root, "static", "images", "hero.png")); got != string(onePNG) {
		t.Error("the bytes written are not the bytes sent")
	}
	// The reply says what landed and what is still missing: an uploaded image
	// nothing points at is not yet a changed picture.
	if !strings.Contains(text(res), "png") || !strings.Contains(text(res), "content_edit") {
		t.Errorf("reply = %q", text(res))
	}

	// It is listed, so the next call can find it without guessing.
	if listing := text(call(t, s, "media_list", map[string]any{})); !strings.Contains(listing, "hero.png") {
		t.Errorf("listing = %q", listing)
	}
}

// TestTheBytesDecideNotTheName. The extension decides nothing — SEC-013 wrote
// that down and #198 was the AVIF pass forgetting it a release later.
func TestTheBytesDecideNotTheName(t *testing.T) {
	s, root := newTestServer(t, nil)

	res := call(t, s, "media_upload", map[string]any{
		"path": "static/images/evil.png", "content_base64": b64([]byte("#!/bin/sh\nrm -rf /\n")),
	})
	if !res.IsError {
		t.Fatal("a shell script named .png must be refused")
	}
	if !strings.Contains(text(res), "own bytes decide") {
		t.Errorf("the refusal must say why: %q", text(res))
	}
	if _, err := os.Stat(filepath.Join(root, "static", "images", "evil.png")); err == nil {
		t.Error("nothing must be written for a refused upload")
	}
}

// TestUploadAndReplaceAreSeparate, for the same reason content_create and
// content_update are: creating over an existing file and replacing a missing
// one are both mistakes, and splitting them turns each into an error.
func TestUploadAndReplaceAreSeparate(t *testing.T) {
	s, _ := newTestServer(t, nil)
	up := map[string]any{"path": "static/a.png", "content_base64": b64(onePNG)}

	if res := call(t, s, "media_upload", up); res.IsError {
		t.Fatalf("first upload: %s", text(res))
	}
	if res := call(t, s, "media_upload", up); !res.IsError ||
		!strings.Contains(text(res), "media_replace") {
		t.Errorf("a second upload to the same path must be refused: %q", text(res))
	}
	if res := call(t, s, "media_replace", up); res.IsError {
		t.Errorf("replace must succeed: %s", text(res))
	}
	if res := call(t, s, "media_replace", map[string]any{
		"path": "static/absent.png", "content_base64": b64(onePNG),
	}); !res.IsError || !strings.Contains(text(res), "media_upload") {
		t.Errorf("replacing a missing file must be refused: %q", text(res))
	}
}

// TestMediaCannotEscapeItsDirectories: the confinement the other tools enforce
// must not have a hole where the binary writes are.
func TestMediaCannotEscapeItsDirectories(t *testing.T) {
	s, _ := newTestServer(t, nil)
	for _, path := range []string{"../escape.png", "/etc/passwd", "content/posts/x.png", "templates/x.png"} {
		if res := call(t, s, "media_upload", map[string]any{
			"path": path, "content_base64": b64(onePNG),
		}); !res.IsError {
			t.Errorf("%q must be refused", path)
		}
	}
}

// TestDeletingAPictureThreePagesUseIsRefused. A broken image is a worse
// outcome than an unused file, and the refusal has to name what would break.
func TestDeletingAPictureThreePagesUseIsRefused(t *testing.T) {
	s, root := newTestServer(t, nil)
	if res := call(t, s, "media_upload", map[string]any{
		"path": "static/images/logo.png", "content_base64": b64(onePNG),
	}); res.IsError {
		t.Fatal(text(res))
	}
	// A page referring to it the way a site serves it, not the way it is stored.
	writeProjectFile(t, root, "content/posts/hello.md", "---\ntitle: T\n---\n\n![logo](/images/logo.png)\n")

	res := call(t, s, "media_delete", map[string]any{"path": "static/images/logo.png"})
	if !res.IsError {
		t.Fatal("deleting a referenced image must be refused")
	}
	if !strings.Contains(text(res), "hello.md") {
		t.Errorf("the refusal must name what still points at it: %q", text(res))
	}
	if !fileExists(filepath.Join(root, "static", "images", "logo.png")) {
		t.Error("a refused delete must leave the file alone")
	}

	// Once nothing points at it, it goes.
	writeProjectFile(t, root, "content/posts/hello.md", "---\ntitle: T\n---\n\nNo image now.\n")
	if res := call(t, s, "media_delete", map[string]any{"path": "static/images/logo.png"}); res.IsError {
		t.Fatalf("an unreferenced image must be deletable: %s", text(res))
	}
	if fileExists(filepath.Join(root, "static", "images", "logo.png")) {
		t.Error("the file is still there")
	}
}

// TestAURLIsDownloadedThroughTheGuard: the owner pastes a link, which the
// ticket calls the case that matters most.
func TestAURLIsDownloadedThroughTheGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(onePNG)
	}))
	defer srv.Close()

	// httptest listens on loopback, which the guard refuses by design — so this
	// asserts the refusal, and the allowed path is asserted with the flag on.
	s, _ := newTestServer(t, nil)
	res := call(t, s, "media_upload", map[string]any{"path": "static/a.png", "url": srv.URL})
	if !res.IsError {
		t.Fatal("a URL resolving to loopback must be refused by default")
	}

	allowed, root := newTestServer(t, func(o *Options) { o.MediaAllowPrivate = true })
	if res := call(t, allowed, "media_upload", map[string]any{
		"path": "static/a.png", "url": srv.URL,
	}); res.IsError {
		t.Fatalf("with the opt-in the download must succeed: %s", text(res))
	}
	if got := readProjectFile(t, filepath.Join(root, "static", "a.png")); got != string(onePNG) {
		t.Error("the downloaded bytes are not what was written")
	}
}

// TestOnlyHTTPURLsAreFetched: file:// would read the server's disk, and a
// scheme nobody thought about is how that happens.
func TestOnlyHTTPURLsAreFetched(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) { o.MediaAllowPrivate = true })
	for _, u := range []string{"file:///etc/passwd", "gopher://x/", "ftp://x/y.png", "://broken"} {
		if res := call(t, s, "media_upload", map[string]any{
			"path": "static/a.png", "url": u,
		}); !res.IsError {
			t.Errorf("%q must be refused", u)
		}
	}
}

// TestOneSourceOrTheOther, so a call carrying both is a mistake named rather
// than a silent choice between them.
func TestOneSourceOrTheOther(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if res := call(t, s, "media_upload", map[string]any{
		"path": "static/a.png", "content_base64": b64(onePNG), "url": "https://example.com/a.png",
	}); !res.IsError || !strings.Contains(text(res), "not both") {
		t.Errorf("result = %q", text(res))
	}
	if res := call(t, s, "media_upload", map[string]any{"path": "static/a.png"}); !res.IsError {
		t.Error("an upload with no source must be refused")
	}
	if res := call(t, s, "media_upload", map[string]any{
		"path": "static/a.png", "content_base64": "not base64!!",
	}); !res.IsError || !strings.Contains(text(res), "base64") {
		t.Errorf("result = %q", text(res))
	}
}

// TestAnOversizedFileIsRefusedRatherThanTruncated.
func TestAnOversizedFileIsRefusedRatherThanTruncated(t *testing.T) {
	huge := append(append([]byte{}, onePNG...), make([]byte, maxMediaBytes)...)
	if _, err := mediaKind(huge); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("err = %v", err)
	}
	if _, err := mediaKind(nil); err == nil {
		t.Error("an empty file must be refused")
	}
}

// TestMediaToolsAreOfferedToBothRoles: a photograph is neither purely content
// nor purely presentation, and the owner asking does not know the difference.
func TestMediaToolsAreOfferedToBothRoles(t *testing.T) {
	for _, role := range []string{"designer", "content"} {
		s, _ := newTestServer(t, func(o *Options) { o.Roles = map[string]bool{role: true} })
		names := map[string]bool{}
		for _, tl := range s.tools {
			names[tl.name] = true
		}
		for _, want := range []string{"media_list", "media_upload", "media_replace", "media_delete"} {
			if !names[want] {
				t.Errorf("role %q does not get %s", role, want)
			}
		}
	}
}

// TestEveryStorableFormatIsRecognised, from its own magic bytes.
func TestEveryStorableFormatIsRecognised(t *testing.T) {
	cases := map[string][]byte{
		"jpeg": {0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'},
		"png":  onePNG,
		"gif":  []byte("GIF89a" + strings.Repeat("\x00", 8)),
		"webp": append(append([]byte("RIFF"), 0, 0, 0, 0), []byte("WEBPVP8 ")...),
	}
	for want, data := range cases {
		got, err := mediaKind(data)
		if err != nil || got != want {
			t.Errorf("mediaKind(%s) = %q, %v", want, got, err)
		}
	}
	// And the ones deliberately outside the set.
	for name, data := range map[string][]byte{
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"pdf":  []byte("%PDF-1.7\n"),
		"html": []byte("<!DOCTYPE html><html>"),
		"gif7": []byte("GIF87"), // truncated: not long enough to be a GIF
	} {
		if _, err := mediaKind(data); err == nil {
			t.Errorf("%s must not be storable", name)
		}
	}
}

// TestAListingSurvivesWhatItCannotRead: a media directory holds things that are
// not images, and one of them must not end the listing.
func TestAListingSurvivesWhatItCannotRead(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/readme.txt", "not an image")
	writeProjectFile(t, root, "static/notes/deep.txt", "nor this")
	if res := call(t, s, "media_upload", map[string]any{
		"path": "static/real.png", "content_base64": b64(onePNG),
	}); res.IsError {
		t.Fatal(text(res))
	}

	listing := text(call(t, s, "media_list", map[string]any{}))
	if !strings.Contains(listing, "real.png") {
		t.Errorf("the image must be listed: %q", listing)
	}
	if strings.Contains(listing, "readme.txt") {
		t.Errorf("a non-image must not be: %q", listing)
	}

	// An empty media directory says so rather than returning nothing.
	empty, _ := newTestServer(t, nil)
	if got := text(call(t, empty, "media_list", map[string]any{})); !strings.Contains(got, "No images yet") {
		t.Errorf("listing = %q", got)
	}
}

// TestDeleteReportsWhatItCannotDo.
func TestDeleteReportsWhatItCannotDo(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if res := call(t, s, "media_delete", map[string]any{"path": "static/absent.png"}); !res.IsError ||
		!strings.Contains(text(res), "does not exist") {
		t.Errorf("result = %q", text(res))
	}
	if res := call(t, s, "media_delete", map[string]any{"path": "../outside.png"}); !res.IsError {
		t.Error("a path outside the media directories must be refused")
	}
}

// TestAWriteFailureIsReported: the bytes were accepted, so the model believes
// the file landed unless the failure comes back.
func TestAMediaWriteFailureIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	s, root := newTestServer(t, nil)
	abs := writeProjectFile(t, root, "static/locked.png", string(onePNG))
	if err := os.Chmod(abs, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(abs, 0o600) })

	if res := call(t, s, "media_replace", map[string]any{
		"path": "static/locked.png", "content_base64": b64(onePNG),
	}); !res.IsError || !strings.Contains(text(res), "write failed") {
		t.Errorf("result = %q", text(res))
	}
}

// TestADownloadThatFailsIsReportedWithoutLeakingTheQueryString: a URL can carry
// a token, and an error message is the least controlled place it could land.
func TestADownloadThatFailsIsReportedWithoutLeakingTheQueryString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s, _ := newTestServer(t, func(o *Options) { o.MediaAllowPrivate = true })
	res := call(t, s, "media_upload", map[string]any{
		"path": "static/a.png", "url": srv.URL + "/missing.png?token=SECRETVALUE",
	})
	if !res.IsError || !strings.Contains(text(res), "404") {
		t.Fatalf("result = %q", text(res))
	}
	if strings.Contains(text(res), "SECRETVALUE") {
		t.Errorf("the query string must be redacted in errors: %q", text(res))
	}
}

// TestReferenceScanningSurvivesAnUnreadableFile: a delete refused because one
// file could not be read would be wrong, and one allowed because of it would be
// worse. It skips what it cannot read and keeps looking.
func TestReferenceScanningSurvivesAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	s, root := newTestServer(t, nil)
	locked := writeProjectFile(t, root, "content/posts/locked.md", "---\ntitle: L\n---\n")
	writeProjectFile(t, root, "content/posts/uses.md", "---\ntitle: U\n---\n\n![x](/images/pic.png)\n")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	if used := s.mediaReferences("static/images/pic.png"); len(used) != 1 ||
		!strings.Contains(used[0], "uses.md") {
		t.Errorf("references = %v, want only the readable file that points at it", used)
	}
}

// TestAMediaDirectoryThatIsNotThereIsAnOrdinarySite.
func TestAMediaDirectoryThatIsNotThereIsAnOrdinarySite(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) { o.StaticDirs = []string{"no-such-dir"} })
	if got := text(call(t, s, "media_list", map[string]any{})); !strings.Contains(got, "No images yet") {
		t.Errorf("listing = %q", got)
	}
}

// TestWritingIntoAPathThatCannotBeCreatedIsReported.
func TestWritingIntoAPathThatCannotBeCreatedIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s, root := newTestServer(t, nil)
	dir := filepath.Join(root, "static", "ro")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if res := call(t, s, "media_upload", map[string]any{
		"path": "static/ro/deep/a.png", "content_base64": b64(onePNG),
	}); !res.IsError || !strings.Contains(text(res), "write failed") {
		t.Errorf("result = %q", text(res))
	}
}
