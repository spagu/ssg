package generator

// Which ssg built the site, and how big and how fast the build was (#338).
//
// `.SSG.Version` is in every template context that has `.BuildTime`, so a
// footer or a <meta name="generator"> can say which ssg built the page. It is
// known before rendering, so it is plain HTML in the page.
//
// The build's size and duration cannot be: a page is written before the build
// ends. `build_report: build.json` (or --build-report) writes them to a file
// next to the pages once the last one is out, before the post_build hooks, so
// a hook can read it too. Sites used to reimplement this in shell — time the
// build, parse `ssg --version`, count *.html with find — and fetch the result
// from every page.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// DefaultBuildReport is the file a bare --build-report writes.
const DefaultBuildReport = "build.json"

// SSGInfo is `.SSG` in templates.
type SSGInfo struct {
	// Version is the ssg that built the site: "1.8.74", "1.8.74+dirty" for a
	// build of a modified checkout, "dev" when nothing better is known.
	Version string
}

// ssgInfo is this build's `.SSG`.
func (g *Generator) ssgInfo() SSGInfo {
	return SSGInfo{Version: g.config.Version}
}

// buildReport is the build report file.
type buildReport struct {
	Generator string `json:"generator"`
	Version   string `json:"version"`
	// BuildTime is `.BuildTime`: SOURCE_DATE_EPOCH when set, so it can differ
	// from Started, which is always the clock.
	BuildTime       string         `json:"buildTime"`
	Started         string         `json:"started"`
	Finished        string         `json:"finished"`
	DurationSeconds float64        `json:"durationSeconds"`
	Pages           int            `json:"pages"` // .html files written
	Files           int            `json:"files"` // every file written, this report excluded
	Content         map[string]int `json:"content"`
}

// writeBuildReport writes the report when one is configured. started is when
// Generate began; now is the clock, a parameter so a test can pin it.
func (g *Generator) writeBuildReport(started time.Time, now func() time.Time) error {
	name := strings.TrimSpace(g.config.BuildReport)
	if name == "" {
		return nil
	}
	dest := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.TrimLeft(name, "/")))
	if err := g.ensureWithinOutput(dest); err != nil {
		return err
	}
	pages, files, err := countOutput(g.config.OutputDir, dest)
	if err != nil {
		return err
	}
	finished := now()
	report := buildReport{
		Generator:       "ssg",
		Version:         g.config.Version,
		BuildTime:       g.buildTime.UTC().Format(time.RFC3339),
		Started:         started.UTC().Format(time.RFC3339),
		Finished:        finished.UTC().Format(time.RFC3339),
		DurationSeconds: float64(finished.Sub(started).Milliseconds()) / 1000,
		Pages:           pages,
		Files:           files,
		Content:         g.contentCounts(),
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	return os.WriteFile(dest, append(raw, '\n'), 0o600)
}

// countOutput counts the .html files and all files under dir, leaving out
// the report itself (a rebuild's previous copy).
func countOutput(dir, report string) (pages, files int, err error) {
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || p == report {
			return nil
		}
		files++
		if strings.EqualFold(filepath.Ext(p), ".html") {
			pages++
		}
		return nil
	})
	return pages, files, err
}

// contentCounts counts the loaded content by type ("post", "page", or a
// custom type), the per-type breakdown a report reader asks for first.
func (g *Generator) contentCounts() map[string]int {
	counts := map[string]int{}
	if g.siteData == nil {
		return counts
	}
	for _, list := range [][]models.Page{g.siteData.Posts, g.siteData.Pages} {
		for _, p := range list {
			counts[contentType(p)]++
		}
	}
	return counts
}

// contentType is a page's type as a report names it.
func contentType(p models.Page) string {
	if t := strings.ToLower(strings.TrimSpace(p.Type)); t != "" {
		return t
	}
	return "page"
}
