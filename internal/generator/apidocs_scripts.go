package generator

// The browser side of API docs: live examples (GO-116) and the REST "Try it"
// console (GO-118). Pages carry only markers — data-ssg-playground and
// data-ssg-tryit — so they work in every theme, and in themes made before
// the feature; the build adds the script and the styles to the pages that
// have a marker, once, before </body>.

import (
	_ "embed"
	"strings"
)

//go:embed assets/playground.js
var playgroundScript string

//go:embed assets/tryit.js
var tryItScript string

//go:embed assets/apitools.css
var apiToolsStyle string

// apiTool is one marker and the script it needs.
type apiTool struct {
	marker, scriptAttr, script string
}

var apiTools = []apiTool{
	{"data-ssg-playground", "data-ssg-playground-script", playgroundScript},
	{"data-ssg-tryit", "data-ssg-tryit-script", tryItScript},
	{"data-ssg-rest", "", ""}, // REST pages: the method labels need only the styles
}

// apiToolsStyleAttr marks the injected stylesheet.
const apiToolsStyleAttr = "data-ssg-api-tools"

// injectAPITools adds the scripts a page's markers need, and the shared
// styles with the first of them. A page without a marker is unchanged.
func injectAPITools(html string) string {
	var body strings.Builder
	needed := false
	for _, t := range apiTools {
		if !strings.Contains(html, t.marker+" ") && !strings.Contains(html, t.marker+">") && !strings.Contains(html, t.marker+"=") {
			continue
		}
		needed = true
		if t.script == "" || strings.Contains(html, t.scriptAttr) {
			continue
		}
		body.WriteString("<script " + t.scriptAttr + ">" + t.script + "</script>")
	}
	add := body.String()
	if needed && !strings.Contains(html, apiToolsStyleAttr) {
		add = "<style " + apiToolsStyleAttr + ">" + apiToolsStyle + "</style>" + add
	}
	if add == "" {
		return html
	}
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		return html[:i] + add + "\n" + html[i:]
	}
	return html + add
}
