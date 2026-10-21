//go:build !windows

package generator

// Everything a build writes into the output directory is published, so a web
// server running as another user must be able to read every file and enter
// every directory (#342). The report shipped 0600 and the component assets'
// directory 0750; both answered 403 behind nginx. This builds a site with the
// features that write their own files turned on and checks the whole tree,
// so the next feature that picks a private mode fails here, not in production.

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask runs fn under a conventional umask, so the test checks the modes
// ssg asks for rather than the CI machine's setting.
func withUmask(t *testing.T, mask int, fn func()) {
	t.Helper()
	old := syscall.Umask(mask)
	defer syscall.Umask(old)
	fn()
}

func TestEveryOutputFileIsWorldReadable(t *testing.T) {
	cfg := componentSite(t, map[string]string{
		"pages/demo.md": "---\ntitle: Demo\nslug: demo\nstatus: publish\ntype: page\n---\n\n{{< youtube id=\"abc\" >}}\n",
		"posts/one.md":  "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2025-01-02\ntags: [go]\n---\n\nBody.\n",
	}, func(c *Config) {
		c.BuildReport = "meta/build.json" // #342: the report and a directory of its own
		c.SiteGraph = true
		c.RouteManifest = true
		c.SearchIndex = true
		c.Feed = true
		c.SEO = true
	})
	withUmask(t, 0o022, func() { buildSiteFixture(t, cfg) })

	checked := 0
	err := filepath.WalkDir(cfg.OutputDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		rel, _ := filepath.Rel(cfg.OutputDir, p)
		switch {
		case d.IsDir() && perm&0o005 != 0o005:
			t.Errorf("directory %s is %v: a web server running as another user cannot enter it", rel, perm)
		case !d.IsDir() && perm&0o004 == 0:
			t.Errorf("file %s is %v: a web server running as another user cannot read it", rel, perm)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The features above must actually have written their files, or the walk
	// proves nothing about them.
	for _, rel := range []string{"meta/build.json", "components/youtube/youtube.css", "components.json",
		"sitemap.xml", "robots.txt", "search-index.json", "routes.json", "site-graph.json"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, rel)); err != nil {
			t.Errorf("%s was not written; the check did not cover it", rel)
		}
	}
	if checked < 10 {
		t.Errorf("only %d entries checked", checked)
	}
}
