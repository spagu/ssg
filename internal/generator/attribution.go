package generator

// The "Built with ssg" notice (1.9.0).
//
// From 1.9.0 ssg is licensed under the Business Source License 1.1. Its
// Additional Use Grant allows production use at no charge on one condition:
// every generated site keeps a visible notice naming ssg. This is where the
// notice is written, so a site built with the defaults complies without the
// theme author doing anything.
//
// A theme that places its own notice marks it with data-ssg-attribution (the
// bundled apidoc and ssgtheme do), and nothing is added. `attribution: false`
// removes it; that setting is allowed under a commercial license only, see
// docs/LICENSING.md.

import "strings"

// attributionURL is the project page the notice links to.
const attributionURL = "https://ssg.tradik.com/"

// attributionMarker is the attribute a theme puts on its own notice.
const attributionMarker = "data-ssg-attribution"

// attributionOn reports whether the notice is written; the default is yes.
func (g *Generator) attributionOn() bool {
	return g.config.Attribution == nil || *g.config.Attribution
}

// injectAttribution adds the generator meta tag to <head> and the visible
// notice before </body>, each only where the page lacks it. A redirect stub
// (meta refresh) gets the meta tag only: nobody reads it.
func (g *Generator) injectAttribution(s string) string {
	if !g.attributionOn() {
		return s
	}
	// The name alone, no version: the golden corpora and a site's own diffs
	// must not change with every release or between a dev build and CI. The
	// version is in build.json and .SSG for whoever wants it.
	if !strings.Contains(s, `name="generator"`) {
		if i := strings.LastIndex(s, "</head>"); i >= 0 {
			s = s[:i] + `<meta name="generator" content="ssg">` + "\n" + s[i:]
		}
	}
	if strings.Contains(s, attributionMarker) || strings.Contains(s, `href="`+attributionURL) ||
		strings.Contains(s, `http-equiv="refresh"`) {
		return s
	}
	i := strings.LastIndex(s, "</body>")
	if i < 0 {
		return s
	}
	notice := `<p class="ssg-attribution" ` + attributionMarker + `>Built with <a href="` + attributionURL + `" rel="generator">ssg</a></p>` + "\n"
	return s[:i] + notice + s[i:]
}
