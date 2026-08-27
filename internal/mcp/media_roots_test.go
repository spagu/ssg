package mcp

// Media addressed the way the site serves them (#218).

import (
	"path/filepath"
	"strings"
	"testing"
)

// migratedServer is the reported shape: a site out of wpexporter, whose
// pictures live under the content source and are served at /media/, with a
// static/ that holds the theme's own and little else.
func migratedServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, root := newTestServer(t, func(o *Options) {
		o.MediaRoots = []MediaRoot{
			{Dir: "static", URL: "/"},
			{Dir: "content/site/media", URL: "/media/"},
		}
	})
	writeProjectFile(t, root, "content/site/media/images/team.jpg", string(oneJPEG))
	writeProjectFile(t, root, "static/logo.png", string(onePNG))
	return s, root
}

// oneJPEG is a minimal JPEG: enough of a header to be recognised.
var oneJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0}, make([]byte, 16)...)

// TestAMigratedSiteSeesItsOwnPictures — the reported case. media_list said
// "nothing" while the site served hundreds, because it knew only static/.
func TestAMigratedSiteSeesItsOwnPictures(t *testing.T) {
	s, _ := migratedServer(t)

	listing := text(call(t, s, "media_list", map[string]any{}))
	// Reported the way the site addresses them, not the way they are stored:
	// the owner knows the picture on the about page, not which root holds it.
	if !strings.Contains(listing, "/media/images/team.jpg") {
		t.Errorf("the migrated pictures must be listed by served path:\n%s", listing)
	}
	if !strings.Contains(listing, "/logo.png") {
		t.Errorf("the theme's own must still be listed:\n%s", listing)
	}
	if strings.Contains(listing, "content/site/media") {
		t.Errorf("the stored path is not an address anyone uses:\n%s", listing)
	}
}

// TestReplacingAMigratedPictureIsTheOneCallThatMatters: keeping the path means
// every page using it changes at once, with no content edits — which is what
// "change the picture on the about page" means.
func TestReplacingAMigratedPictureIsTheOneCallThatMatters(t *testing.T) {
	s, root := migratedServer(t)

	res := call(t, s, "media_replace", map[string]any{
		"path": "/media/images/team.jpg", "content_base64": b64(onePNG),
	})
	if res.IsError {
		t.Fatalf("replace by served path failed: %s", text(res))
	}
	// It replaced the file where it actually lives.
	if got := readProjectFile(t, filepath.Join(root, "content", "site", "media", "images", "team.jpg")); got != string(onePNG) {
		t.Error("the file under the content source was not replaced")
	}
	// And wrote nothing into static/.
	if fileExists(filepath.Join(root, "static", "media", "images", "team.jpg")) {
		t.Error("a copy leaked into the theme's directory")
	}
}

// TestANewPictureLandsInTheRootThatServesIt: a file addressed under /media/
// must not go to static/, or a migrated site ends up with two media trees.
func TestANewPictureLandsInTheRootThatServesIt(t *testing.T) {
	s, root := migratedServer(t)

	if res := call(t, s, "media_upload", map[string]any{
		"path": "/media/images/new.png", "content_base64": b64(onePNG),
	}); res.IsError {
		t.Fatalf("upload: %s", text(res))
	}
	if !fileExists(filepath.Join(root, "content", "site", "media", "images", "new.png")) {
		t.Error("a /media/ upload must land under the content source")
	}

	if res := call(t, s, "media_upload", map[string]any{
		"path": "/theme.png", "content_base64": b64(onePNG),
	}); res.IsError {
		t.Fatalf("upload: %s", text(res))
	}
	if !fileExists(filepath.Join(root, "static", "theme.png")) {
		t.Error("a root-level upload must land in static/")
	}
}

// TestTheLongestPrefixWins: every root is under "/" in the sense that "/" is a
// prefix of everything, so a first-match search would send every /media/ upload
// into the theme's directory.
func TestTheLongestPrefixWins(t *testing.T) {
	roots := rootsBySpecificity([]MediaRoot{
		{Dir: "static", URL: "/"},
		{Dir: "content/site/media", URL: "/media/"},
		{Dir: "assets", URL: "/assets/deep/"},
	})
	if roots[0].URL != "/assets/deep/" || roots[len(roots)-1].URL != "/" {
		t.Errorf("order = %+v, want most specific first", roots)
	}
}

// TestAStoredPathStillWorks: the tools took project-relative paths when they
// shipped a release ago, and an agent that learned them then should not be told
// its own path is wrong today.
func TestAStoredPathStillWorks(t *testing.T) {
	s, root := migratedServer(t)

	if res := call(t, s, "media_replace", map[string]any{
		"path": "content/site/media/images/team.jpg", "content_base64": b64(onePNG),
	}); res.IsError {
		t.Fatalf("a stored path must still resolve: %s", text(res))
	}
	if got := readProjectFile(t, filepath.Join(root, "content", "site", "media", "images", "team.jpg")); got != string(onePNG) {
		t.Error("the file was not replaced")
	}
}

// TestAPathBelongingToNoRootIsRefusedWithSomewhereToLook.
func TestAPathBelongingToNoRootIsRefusedWithSomewhereToLook(t *testing.T) {
	s, _ := migratedServer(t)

	res := call(t, s, "media_upload", map[string]any{
		"path": "content/site/posts/x.png", "content_base64": b64(onePNG),
	})
	if !res.IsError {
		t.Fatal("a path under no publishing root must be refused")
	}
	for _, want := range []string{"/media/", "media_list"} {
		if !strings.Contains(text(res), want) {
			t.Errorf("the refusal must say where to look, missing %q: %s", want, text(res))
		}
	}
}

// TestDeleteKnowsWhichServedPathAPageUses: the reference check guessed the
// served form by chopping the first path segment, which is right for static/
// and wrong for a content source — so a /media/ picture three pages used
// looked unreferenced.
func TestDeleteKnowsWhichServedPathAPageUses(t *testing.T) {
	s, root := migratedServer(t)
	writeProjectFile(t, root, "content/site/pages/about.md",
		"---\ntitle: About\n---\n\n![team](/media/images/team.jpg)\n")

	res := call(t, s, "media_delete", map[string]any{"path": "/media/images/team.jpg"})
	if !res.IsError {
		t.Fatal("a referenced picture must not be deletable")
	}
	if !strings.Contains(text(res), "about.md") {
		t.Errorf("the refusal must name the page: %s", text(res))
	}
}

// TestWithoutConfiguredRootsTheOldBehaviourStands, so a caller that never set
// MediaRoots keeps exactly what it had.
func TestWithoutConfiguredRootsTheOldBehaviourStands(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/a.png", string(onePNG))

	if got := text(call(t, s, "media_list", map[string]any{})); !strings.Contains(got, "/a.png") {
		t.Errorf("listing = %q", got)
	}
	if res := call(t, s, "media_replace", map[string]any{
		"path": "static/a.png", "content_base64": b64(oneJPEG),
	}); res.IsError {
		t.Errorf("a stored path under StaticDirs must resolve: %s", text(res))
	}
}
