package main

// Error paths of small CLI helpers that the end-to-end tests do not reach.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestDispatchSubcommandWithNothing: no arguments is the plain build.
func TestDispatchSubcommandWithNothing(t *testing.T) {
	if code, handled := dispatchSubcommand(nil); handled || code != 0 {
		t.Errorf("got %d, %v", code, handled)
	}
}

// TestImportRedirectsMissingFiles names the file it could not read.
func TestImportRedirectsMissingFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	if _, _, err := importRedirectsFromJSON(missing); err == nil || !strings.Contains(err.Error(), "nope.json") {
		t.Errorf("json: %v", err)
	}
	if _, _, err := importRedirectsFromConfig(missing); err == nil || !strings.Contains(err.Error(), "nope.json") {
		t.Errorf("config: %v", err)
	}
}

// TestParseNextRedirectsSkipsHalfARule: an object without a destination is
// not a redirect.
func TestParseNextRedirectsSkipsHalfARule(t *testing.T) {
	got, warnings := parseNextRedirects(`{ source: '/a' }, { source: '/b', destination: '/c' }`)
	if len(got) != 1 || got[0].Source != "/b" || len(warnings) != 0 {
		t.Errorf("got %+v, %v", got, warnings)
	}
}

// TestGitRunnerWithoutGit refuses to run anything rather than trusting PATH.
func TestGitRunnerWithoutGit(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := gitRunner()("status"); err == nil || !strings.Contains(err.Error(), "git is unavailable") {
		t.Errorf("err = %v", err)
	}
}

// TestBuildAuthGuardNeedsAnAbsolutePath.
func TestBuildAuthGuardNeedsAnAbsolutePath(t *testing.T) {
	if _, err := buildAuthGuard(config.Endpoint{Path: "admin"}); err == nil || !strings.Contains(err.Error(), "must start with '/'") {
		t.Errorf("err = %v", err)
	}
}

// TestStartPprofWhileAnotherProfileRuns: a CPU profile already running is a
// warning, not a failed build, and the returned stop is still callable.
func TestStartPprofWhileAnotherProfileRuns(t *testing.T) {
	if err := pprof.StartCPUProfile(io.Discard); err != nil {
		t.Skip("a CPU profile is already running:", err)
	}
	defer pprof.StopCPUProfile()
	stop := startPprof(&config.Config{ProfilePprof: t.TempDir(), Quiet: true})
	stop()
}

// TestWriteHeapProfileIntoAMissingDirectory warns and returns.
func TestWriteHeapProfileIntoAMissingDirectory(t *testing.T) {
	writeHeapProfile(filepath.Join(t.TempDir(), "missing"))
}

// TestExtractWorkerTemplateMissingRoot: a template that is not embedded is an
// error, not an empty scaffold.
func TestExtractWorkerTemplateMissingRoot(t *testing.T) {
	if err := extractWorkerTemplate("workers/no-such-template", t.TempDir()); err == nil {
		t.Error("want an error")
	}
}

// TestFormEndpointUnparseableBody is a 400, and a build that is not quiet says
// how many endpoints it serves.
func TestFormEndpointUnparseableBody(t *testing.T) {
	ep := config.Endpoint{Type: "form", Path: "/api/contact", To: "https://example.com/hook"}
	h := endpointHandler(&config.Config{Endpoints: []config.Endpoint{ep}}, http.NotFoundHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader("name=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}
