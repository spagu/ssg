package main

// `ssg mddb push-theme` and the MDDB search backend (#190).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mddb"
)

// fakeMddb records what a sync sent it and can pretend a collection already
// holds documents.
type fakeMddb struct {
	mu       sync.Mutex
	added    []mddb.AddRequest
	deleted  []string
	existing []string
	failAdd  bool
	failList bool
	// validation (#192)
	validated           int
	warnings            string
	validationErrors    string
	validateUnsupported bool
}

// validOrNot and errsOrNone let one fake answer both shapes: a document the
// schema accepts, and one it does not.
func (f *fakeMddb) validOrNot() string {
	if f.validationErrors != "" {
		return "false"
	}
	return "true"
}

func (f *fakeMddb) errsOrNone() string {
	if f.validationErrors != "" {
		return f.validationErrors
	}
	return "[]"
}

func (f *fakeMddb) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.warnings == "" {
			f.warnings = "[]"
		}
		switch r.URL.Path {
		case "/v1/validate":
			f.validated++
			if f.validateUnsupported {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"valid":` + f.validOrNot() + `,"errors":` + f.errsOrNone() +
				`,"warnings":` + f.warnings + `}`))
		case "/v1/add":
			if f.failAdd {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var req mddb.AddRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.added = append(f.added, req)
		case "/v1/delete":
			var req mddb.DeleteRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.deleted = append(f.deleted, req.Key)
		case "/v1/search":
			if f.failList {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			docs := make([]map[string]any, 0, len(f.existing))
			for _, k := range f.existing {
				docs = append(docs, map[string]any{"key": k, "contentMd": "old"})
			}
			f.existing = nil // one page, then empty: the pager stops
			w.Header().Set("X-Total-Count", "0")
			_ = json.NewEncoder(w).Encode(docs)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// themeProject lays out a project with a theme and points its config at srv.
func themeProject(t *testing.T, srvURL string) {
	t.Helper()
	t.Chdir(t.TempDir())
	for path, body := range map[string]string{
		"templates/simple/base.html":   "<html></html>",
		"templates/simple/css/app.css": "body{}",
		"static/js/main.js":            "console.log(1)",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	yaml := "source: s\ntemplate: simple\ndomain: example.com\nmcp:\n  search:\n" +
		"    mddb_url: " + srvURL + "\n    mddb_collection: theme\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPushThemeUploadsEveryFileWithItsIdentity: the sync is what makes a
// natural-language search over the theme possible at all.
func TestPushThemeUploadsEveryFileWithItsIdentity(t *testing.T) {
	f := &fakeMddb{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme(nil); code != 0 {
		t.Fatalf("push-theme = %d", code)
	}
	if len(f.added) != 3 {
		t.Fatalf("pushed %d file(s), want 3: %+v", len(f.added), f.added)
	}
	byKey := map[string]mddb.AddRequest{}
	for _, a := range f.added {
		byKey[a.Key] = a
	}
	css, ok := byKey["templates/simple/css/app.css"]
	if !ok {
		t.Fatalf("stylesheet not pushed: %v", byKey)
	}
	// The key is the project-relative path, so a search hit names the file to
	// open — that is what makes a hit actionable rather than merely relevant.
	if css.ContentMD != "body{}" {
		t.Errorf("content = %q", css.ContentMD)
	}
	if css.Meta["kind"][0] != "style" {
		t.Errorf("kind = %v", css.Meta["kind"])
	}
	if css.Meta["path"][0] != "templates/simple/css/app.css" {
		t.Errorf("path = %v", css.Meta["path"])
	}
	if len(css.Meta["checksum"][0]) != 64 {
		t.Errorf("checksum = %v", css.Meta["checksum"])
	}
	if css.Meta["size"][0] != "6" {
		t.Errorf("size = %v", css.Meta["size"])
	}
	if byKey["static/js/main.js"].Meta["kind"][0] != "script" {
		t.Errorf("js kind = %v", byKey["static/js/main.js"].Meta["kind"])
	}
	if byKey["templates/simple/base.html"].Meta["kind"][0] != "template" {
		t.Errorf("html kind = %v", byKey["templates/simple/base.html"].Meta["kind"])
	}
}

// TestPushThemeRemovesWhatVanished: the sync is a reconciliation. A deleted
// partial that stays in the index sends the agent to a file that is not there.
func TestPushThemeRemovesWhatVanished(t *testing.T) {
	f := &fakeMddb{existing: []string{"templates/simple/base.html", "templates/simple/gone.html"}}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme(nil); code != 0 {
		t.Fatalf("push-theme = %d", code)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "templates/simple/gone.html" {
		t.Errorf("deleted = %v, want only the vanished file", f.deleted)
	}
}

// TestADryRunTouchesNothing, so the command can be checked before it is trusted.
func TestADryRunTouchesNothing(t *testing.T) {
	f := &fakeMddb{existing: []string{"gone.css"}}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme([]string{"--dry"}); code != 0 {
		t.Fatalf("dry run = %d", code)
	}
	if len(f.added) != 0 || len(f.deleted) != 0 {
		t.Errorf("a dry run wrote: added %v, deleted %v", f.added, f.deleted)
	}
}

// TestPushThemeWithoutAConfiguredTargetSaysWhatToSet, rather than failing at
// the first request with a connection error.
func TestPushThemeWithoutAConfiguredTargetSaysWhatToSet(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".ssg.yaml",
		[]byte("source: s\ntemplate: simple\ndomain: example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
	})
	if !strings.Contains(out, "mcp.search.mddb_url") {
		t.Errorf("stderr = %q", out)
	}
}

// TestAFailedUploadIsReportedAndExitsNonZero: a sync that half-landed and said
// nothing leaves an index that is wrong in a way nobody can see.
func TestAFailedUploadIsReportedAndExitsNonZero(t *testing.T) {
	f := &fakeMddb{failAdd: true}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
	})
	if !strings.Contains(out, "failed") {
		t.Errorf("stderr = %q", out)
	}
}

// TestAnUnlistableCollectionStillReportsTheUploads: refusing to acknowledge
// work that landed because the cleanup half failed is worse than saying so.
func TestAnUnlistableCollectionStillReportsTheUploads(t *testing.T) {
	f := &fakeMddb{failList: true}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 0 {
			t.Errorf("code = %d, want 0 — the uploads succeeded", code)
		}
	})
	if !strings.Contains(out, "stale documents") {
		t.Errorf("the prune failure must be reported: %q", out)
	}
	if len(f.added) != 3 {
		t.Errorf("uploads = %d, want 3", len(f.added))
	}
}

// TestAnEmptyThemeIsAnError, not a sync that silently deletes the collection.
func TestAnEmptyThemeIsAnError(t *testing.T) {
	f := &fakeMddb{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	t.Chdir(t.TempDir())
	yaml := "source: s\ntemplate: simple\ndomain: example.com\ntemplates_dir: nope\nstatic_dir: nope2\n" +
		"mcp:\n  search:\n    mddb_url: " + srv.URL + "\n    mddb_collection: theme\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
	})
	if !strings.Contains(out, "no template or asset files") {
		t.Errorf("stderr = %q", out)
	}
	if len(f.deleted) != 0 {
		t.Errorf("an empty theme must not delete anything: %v", f.deleted)
	}
}

// TestUnknownFlagsAreRefusedWithUsage.
func TestUnknownFlagsAreRefusedWithUsage(t *testing.T) {
	out := captureStderr(t, func() {
		if code := runMddbPushTheme([]string{"--nope"}); code != 1 {
			t.Errorf("code = %d", code)
		}
	})
	if !strings.Contains(out, "usage: ssg mddb push-theme") {
		t.Errorf("stderr = %q", out)
	}
}

// TestPushThemeIsDispatched: a subcommand nobody can reach is not a feature.
func TestPushThemeIsDispatched(t *testing.T) {
	if _, handled := dispatchSubcommand([]string{"mddb", "push-theme", "--nope"}); !handled {
		t.Error("`ssg mddb push-theme` must dispatch")
	}
	// The verb+noun rule holds: a source directory named "mddb" still builds.
	if _, handled := dispatchSubcommand([]string{"mddb", "simple", "example.com"}); handled {
		t.Error("`ssg mddb simple example.com` must remain a normal build")
	}
}

// TestThemeKindLabelsWhatItCan, so a query can be narrowed to stylesheets.
func TestThemeKindLabelsWhatItCan(t *testing.T) {
	cases := map[string]string{
		"a.css": "style", "a.SCSS": "style", "a.js": "script", "a.ts": "script",
		"a.html": "template", "a.tmpl": "template", "a.yaml": "data", "a.png": "asset", "a": "asset",
	}
	for path, want := range cases {
		if got := themeKind(path); got != want {
			t.Errorf("themeKind(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestTheSearchBackendIsOptional: nil is the documented "scan locally" case, so
// a project with no MDDB needs no configuration at all.
func TestTheSearchBackendIsOptional(t *testing.T) {
	if buildMCPSearch(config.DefaultConfig(), func(string, ...any) {}) != nil {
		t.Error("no configuration must mean no backend")
	}
	half := config.DefaultConfig()
	half.MCP.Search.MddbURL = "http://127.0.0.1:1"
	if buildMCPSearch(half, func(string, ...any) {}) != nil {
		t.Error("a URL with no collection is not a configured backend")
	}
}

// TestTheSearchBackendMapsHitsToPaths: a hit has to name the file to open, and
// the document key is the path the sync stored it under.
func TestTheSearchBackendMapsHitsToPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/fts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"document":{"key":"static/css/style.css",` +
			`"contentMd":"a\nb\nc\nd\ne\nf\ng\nh","meta":{"path":["static/css/style.css"]}},` +
			`"score":0.9,"highlights":[{"fragment":"body { background: #fff; }","startLine":39,"endLine":43}]}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.MCP.Search.MddbURL = srv.URL
	cfg.MCP.Search.MddbCollection = "theme"
	search := buildMCPSearch(cfg, func(string, ...any) {})
	if search == nil {
		t.Fatal("a configured backend must produce a hook")
	}
	hits, err := search("where is the background", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "static/css/style.css" {
		t.Fatalf("hits = %+v", hits)
	}
	if !strings.Contains(hits[0].Note, "0.9") {
		t.Errorf("the score must be shown: %q", hits[0].Note)
	}
	// The locus is the highlight's own range, not the head of the file (#203).
	if hits[0].From != 39 || hits[0].To != 43 {
		t.Errorf("range = %d-%d, want 39-43", hits[0].From, hits[0].To)
	}
	if !strings.Contains(hits[0].Fragment, "background") {
		t.Errorf("fragment = %q, want the matching text", hits[0].Fragment)
	}
}

// TestTheSearchBackendSurfacesFailures so the find tool can fall back rather
// than report an empty result as "not found".
func TestTheSearchBackendSurfacesFailures(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MCP.Search.MddbURL = "http://127.0.0.1:1"
	cfg.MCP.Search.MddbCollection = "theme"
	if _, err := buildMCPSearch(cfg, func(string, ...any) {})("q", 5); err == nil {
		t.Error("an unreachable backend must return an error")
	}
}

// TestTheLangAndDryRunFlagsAreParsed, in both spellings.
func TestTheLangAndDryRunFlagsAreParsed(t *testing.T) {
	f, err := parsePushThemeFlags([]string{"--dry-run", "--lang=pl"})
	if err != nil || !f.dry || f.lang != "pl" {
		t.Errorf("flags = %+v, err = %v", f, err)
	}
	if f, err := parsePushThemeFlags(nil); err != nil || f.dry || f.lang != "" {
		t.Errorf("no flags = %+v, err = %v", f, err)
	}
}

// TestTheLanguageFallsBackThroughTheConfig: an explicit --lang wins, then the
// configured one, then "en" — a document stored under the wrong language is
// invisible to a search that asks for the right one.
func TestTheLanguageFallsBackThroughTheConfig(t *testing.T) {
	f := &fakeMddb{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme([]string{"--lang=pl"}); code != 0 {
		t.Fatalf("push-theme = %d", code)
	}
	for _, a := range f.added {
		if a.Lang != "pl" {
			t.Fatalf("%s stored as %q, want pl", a.Key, a.Lang)
		}
	}
	if got := firstNonEmpty("", "", "en"); got != "en" {
		t.Errorf("fallback = %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("all-empty = %q", got)
	}
}

// TestAnUnreadableThemeFileIsCountedNotFatal: one locked file must not stop the
// rest of the theme from reaching the index.
func TestAnUnreadableThemeFileIsCountedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	f := &fakeMddb{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)
	if err := os.Chmod(filepath.Join("static", "js", "main.js"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join("static", "js", "main.js"), 0o600) })

	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 1 {
			t.Errorf("code = %d, want 1 — one file failed", code)
		}
	})
	if !strings.Contains(out, "main.js") {
		t.Errorf("the unreadable file must be named: %q", out)
	}
	if len(f.added) != 2 {
		t.Errorf("the other %d file(s) must still be pushed", len(f.added))
	}
}

// TestAFailedDeletionIsReportedAndSkipped, without abandoning the rest.
func TestAFailedDeletionIsReportedAndSkipped(t *testing.T) {
	var mu sync.Mutex
	var deleted, searches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/add":
			return
		case "/v1/delete":
			deleted++
			w.WriteHeader(http.StatusInternalServerError)
		case "/v1/search":
			// One page, then empty. GetAll paginates until a batch comes back
			// empty, so a handler that keeps answering with the same page never
			// terminates — which is exactly what the first version of this test
			// did.
			searches++
			if searches == 1 {
				_, _ = w.Write([]byte(`[{"key":"a-gone.css","contentMd":""},{"key":"b-gone.css","contentMd":""}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() { runMddbPushTheme(nil) })
	if !strings.Contains(out, "removing a-gone.css") {
		t.Errorf("a failed delete must be named: %q", out)
	}
	if deleted != 2 {
		t.Errorf("both deletions must be attempted, got %d", deleted)
	}
}

// TestAThemeDirectoryThatIsNotThereIsSimplyEmpty: a project with no static/ is
// ordinary, not an error.
func TestAThemeDirectoryThatIsNotThereIsSimplyEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("templates", "simple"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("templates", "simple", "a.html"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.TemplatesDir, cfg.StaticDir = "templates", "no-such-dir"

	files := themeFiles(cfg)
	if len(files) != 1 || files[0] != "templates/simple/a.html" {
		t.Errorf("files = %v", files)
	}
	// An unset directory is skipped rather than walked from the root.
	cfg.TemplatesDir, cfg.StaticDir = "", ""
	if files := themeFiles(cfg); len(files) != 0 {
		t.Errorf("files = %v", files)
	}
}

// TestABackendHitWithoutPathMetadataFallsBackToTheKey, which is the path the
// sync used in the first place.
func TestABackendHitWithoutPathMetadataFallsBackToTheKey(t *testing.T) {
	got := documentPath(mddb.Document{Key: "templates/base.html"})
	if got != "templates/base.html" {
		t.Errorf("path = %q", got)
	}
	got = documentPath(mddb.Document{Key: "k", Metadata: map[string]any{"path": ""}})
	if got != "k" {
		t.Errorf("an empty path must fall back to the key, got %q", got)
	}
	got = documentPath(mddb.Document{Key: "k", Metadata: map[string]any{"path": 42}})
	if got != "k" {
		t.Errorf("a non-string path must fall back to the key, got %q", got)
	}
}

// TestTheBackendRespectsTheLimit even when the index returns more.
func TestTheBackendRespectsTheLimit(t *testing.T) {
	hits := []mddb.FTSHit{
		{Document: mddb.Document{Key: "a"}}, {Document: mddb.Document{Key: "b"}},
		{Document: mddb.Document{Key: "c"}},
	}
	if got := docsToFindHits(hits, 2); len(got) != 2 {
		t.Errorf("got %d hits, want 2", len(got))
	}
	// A document shorter than the window is not padded past its own end.
	one := docsToFindHits([]mddb.FTSHit{{Document: mddb.Document{Key: "a", Content: "only"}}}, 5)
	if one[0].To != 1 || one[0].Fragment != "only" {
		t.Errorf("hit = %+v", one[0])
	}
	empty := docsToFindHits([]mddb.FTSHit{{Document: mddb.Document{Key: "a"}}}, 5)
	if empty[0].To != 1 {
		t.Errorf("an empty document must still span one line: %+v", empty[0])
	}
}

// TestAWarningReachesTheProducerAtWriteTime is the reported case: the render
// path already recognises a stringified structure, but it does so at the next
// render — another machine, possibly weeks later, naming a document whose
// author has moved on (#192).
func TestAWarningReachesTheProducerAtWriteTime(t *testing.T) {
	f := &fakeMddb{warnings: `["meta.faq: looks like a Go-stringified list of objects"]`}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 0 {
			t.Errorf("code = %d — a warning must never fail the push", code)
		}
	})
	if !strings.Contains(out, "meta.faq") {
		t.Errorf("the warning must reach the operator: %q", out)
	}
	if f.validated == 0 {
		t.Error("nothing was validated")
	}
	// The documents still landed: a warning is advisory on both sides.
	if len(f.added) != 3 {
		t.Errorf("pushed %d, want 3", len(f.added))
	}
}

// TestAnOlderMddbIsAskedOnce, not once per document — a per-file complaint
// about the server would drown the output it is trying to help.
func TestAnOlderMddbIsAskedOnce(t *testing.T) {
	f := &fakeMddb{validateUnsupported: true}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme(nil); code != 0 {
		t.Fatalf("an older server must not fail the push: %d", code)
	}
	if f.validated != 1 {
		t.Errorf("validated %d time(s), want exactly 1", f.validated)
	}
	if len(f.added) != 3 {
		t.Errorf("pushed %d, want 3", len(f.added))
	}
}

// TestValidationCanBeTurnedOff for a batch that does not want the extra round
// trip: /v1/validate is per document, so a large sync doubles its requests.
func TestValidationCanBeTurnedOff(t *testing.T) {
	f := &fakeMddb{warnings: `["meta.faq: stringified"]`}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	if code := runMddbPushTheme([]string{"--no-validate"}); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if f.validated != 0 {
		t.Errorf("--no-validate still validated %d document(s)", f.validated)
	}
	if len(f.added) != 3 {
		t.Errorf("pushed %d, want 3", len(f.added))
	}
}

// TestAValidationThatCannotRunDoesNotStopTheWrite: the document is fine as far
// as anyone knows, and refusing to store it because a diagnostic failed would
// turn a convenience into an outage (#192).
func TestAValidationThatCannotRunDoesNotStopTheWrite(t *testing.T) {
	var mu sync.Mutex
	var added int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/validate":
			_, _ = w.Write([]byte(`not json at all`))
		case "/v1/add":
			added++
		case "/v1/search":
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() {
		if code := runMddbPushTheme(nil); code != 0 {
			t.Errorf("code = %d — a broken validator must not fail the push", code)
		}
	})
	if !strings.Contains(out, "could not validate") {
		t.Errorf("the failure must be reported: %q", out)
	}
	if added != 3 {
		t.Errorf("added %d document(s), want 3", added)
	}
}

// TestASchemaErrorIsReportedBesideTheWarning: the two are independent on the
// MDDB side, and a producer fixing one still needs to see the other (#192).
func TestASchemaErrorIsReportedBesideTheWarning(t *testing.T) {
	f := &fakeMddb{
		warnings:         `["meta.faq: stringified"]`,
		validationErrors: `["meta.kind: required"]`,
	}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	themeProject(t, srv.URL)

	out := captureStderr(t, func() { runMddbPushTheme(nil) })
	if !strings.Contains(out, "meta.kind: required") {
		t.Errorf("the schema error must be reported: %q", out)
	}
	if !strings.Contains(out, "meta.faq: stringified") {
		t.Errorf("the warning must be reported alongside it: %q", out)
	}
}

// TestEachHighlightIsItsOwnLocus: a document that matched in two places is two
// answers, because the agent's next step anchors an edit in one of them (#203).
func TestEachHighlightIsItsOwnLocus(t *testing.T) {
	hits := []mddb.FTSHit{{
		Document: mddb.Document{Key: "templates/site/base.html", Content: "irrelevant"},
		Score:    0.34,
		Highlights: []mddb.Highlight{
			{Fragment: "© 2007 example.com", StartLine: 39, EndLine: 43},
			{Fragment: "footer nav", StartLine: 58, EndLine: 64},
		},
	}}
	got := docsToFindHits(hits, 10)
	if len(got) != 2 {
		t.Fatalf("got %d loci, want 2: %+v", len(got), got)
	}
	if got[0].From != 39 || got[0].To != 43 || got[1].From != 58 || got[1].To != 64 {
		t.Errorf("ranges = %d-%d and %d-%d", got[0].From, got[0].To, got[1].From, got[1].To)
	}
	for _, h := range got {
		if h.Path != "templates/site/base.html" {
			t.Errorf("path = %q", h.Path)
		}
	}
}

// TestAServerWithoutLineRangesSaysSoRatherThanInventOne. Printing lines 1-N as
// though the match were there is what made the old answer worse than the local
// scan: a real file beside a made-up location.
func TestAServerWithoutLineRangesSaysSoRatherThanInventOne(t *testing.T) {
	hits := []mddb.FTSHit{{
		Document:   mddb.Document{Key: "css/style.css", Content: "first line\nsecond\nthird"},
		Score:      0.47,
		Highlights: []mddb.Highlight{{Fragment: "no line numbers here"}}, // pre-2.12.0
	}}
	got := docsToFindHits(hits, 10)
	if len(got) != 1 {
		t.Fatalf("got %d loci", len(got))
	}
	if !strings.Contains(got[0].Note, "line unknown") {
		t.Errorf("the reply must admit it has no location: %q", got[0].Note)
	}
	if got[0].Fragment != "first line" {
		t.Errorf("fragment = %q, want the document's opening line as a label", got[0].Fragment)
	}
}

// TestAMalformedRangeIsDiscarded rather than reported as a location.
func TestAMalformedRangeIsDiscarded(t *testing.T) {
	hits := []mddb.FTSHit{{
		Document: mddb.Document{Key: "a.css", Content: "x"},
		Highlights: []mddb.Highlight{
			{Fragment: "zero", StartLine: 0, EndLine: 5},
			{Fragment: "backwards", StartLine: 9, EndLine: 4},
			{Fragment: "good", StartLine: 3, EndLine: 7},
		},
	}}
	got := docsToFindHits(hits, 10)
	if len(got) != 1 || got[0].From != 3 || got[0].To != 7 {
		t.Fatalf("got %+v, want only the well-formed range", got)
	}
}

// TestTheLimitCountsLoci, not documents: three matches in one file is three
// answers, and a limit of two must stop at two.
func TestTheLimitCountsLoci(t *testing.T) {
	hits := []mddb.FTSHit{{
		Document: mddb.Document{Key: "a.css"},
		Highlights: []mddb.Highlight{
			{StartLine: 1, EndLine: 2}, {StartLine: 5, EndLine: 6}, {StartLine: 9, EndLine: 10},
		},
	}}
	if got := docsToFindHits(hits, 2); len(got) != 2 {
		t.Errorf("got %d loci, want 2", len(got))
	}
}
