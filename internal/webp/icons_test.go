package webp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iconSite writes a site whose head links a touch icon, a favicon by the
// site's absolute URL, a mask icon and a manifest, and whose body shows a
// photo and the icon PNG as an <img>.
func iconSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html": `<html><head>` +
			`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
			`<link rel="icon" type="image/png" href="https://example.com/favicon-32.png?v=2">` +
			`<link rel=mask-icon href="img/mask.png">` +
			`<link rel="manifest" href="/app/site.webmanifest">` +
			`<link rel="stylesheet" href="/style.css">` +
			`<link rel="preload" href="/photo.jpg">` +
			`<link rel="icon" href="data:image/png;base64,AAAA">` +
			`<link rel="icon">` +
			`</head><body><img src="/photo.jpg"><img src="/apple-touch-icon.png"></body></html>`,
		"app/site.webmanifest": `{"icons":[{"src":"icons/192.png"},{"src":"/icons/512.png"}]}`,
		"manifest.json":        `not json`,
		"apple-touch-icon.png": "PNG", "favicon-32.png": "PNG", "img/mask.png": "PNG",
		"app/icons/192.png": "PNG", "icons/512.png": "PNG", "photo.jpg": "JPG",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestPlatformIcons: every way a site names an icon, and nothing else.
func TestPlatformIcons(t *testing.T) {
	dir := iconSite(t)
	got := platformIcons(dir, "https://example.com/")
	for _, want := range []string{"apple-touch-icon.png", "favicon-32.png", "img/mask.png", "app/icons/192.png", "icons/512.png"} {
		if !got[filepath.Join(dir, filepath.FromSlash(want))] {
			t.Errorf("%s is an icon", want)
		}
	}
	if got[filepath.Join(dir, "photo.jpg")] || len(got) != 5 {
		t.Errorf("icons = %v", got)
	}
	if len(platformIcons(filepath.Join(dir, "missing"), "")) != 0 {
		t.Error("no site, no icons")
	}
}

// TestIconsStayPNG reproduces #392: in both modes the touch icon keeps its
// PNG and its link, while the photo beside it is converted and re-pointed.
func TestIconsStayPNG(t *testing.T) {
	for _, keep := range []bool{false, true} {
		withFakeCwebp(t)
		dir := iconSite(t)
		if _, _, err := ConvertDirectory(dir, ConvertOptions{Quiet: true, KeepOriginal: keep, Origin: "https://example.com"}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "apple-touch-icon.png")); err != nil {
			t.Errorf("keep=%v: the touch icon PNG is gone", keep)
		}
		if _, err := os.Stat(filepath.Join(dir, "apple-touch-icon.webp")); err == nil {
			t.Errorf("keep=%v: the touch icon was converted", keep)
		}
		// A .webp left by an earlier build must not attract the link either.
		if err := os.WriteFile(filepath.Join(dir, "apple-touch-icon.webp"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := UpdateReferencesOnSite(dir, "https://example.com", keep); err != nil {
			t.Fatal(err)
		}
		html := mustReadFile(t, filepath.Join(dir, "index.html"))
		for _, want := range []string{`href="/apple-touch-icon.png"`, `favicon-32.png?v=2`, `href="img/mask.png"`, `<img src="/photo.webp">`, `<img src="/apple-touch-icon.png">`} {
			if !strings.Contains(html, want) {
				t.Errorf("keep=%v: %q missing in\n%s", keep, want, html)
			}
		}
	}
}

func TestIconRelAndUnquote(t *testing.T) {
	for rel, want := range map[string]bool{
		"icon": true, "Shortcut Icon": true, "apple-touch-icon-precomposed": true, "mask-icon": true,
		"manifest": false, "stylesheet": false, "preload": false,
	} {
		if iconRel(rel) != want {
			t.Errorf("iconRel(%q) != %v", rel, want)
		}
	}
	if unquote(`"a"`) != "a" || unquote(`'a'`) != "a" || unquote("a") != "a" || unquote(`"a'`) != `"a'` {
		t.Error("unquote")
	}
}
