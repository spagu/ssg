package generator

import (
	"fmt"

	"github.com/spagu/ssg/internal/apicheck"
)

// checkAPIIfRequested reports documentation problems in api_docs packages
// (GO-110): what the extractors could not read, links to nothing, and the
// model-level gaps apicheck finds. "warn" lists them; "strict" — or a strict
// build — fails with the whole list. Without api_docs it has nothing to do.
func (g *Generator) checkAPIIfRequested() error {
	mode := g.resolveMode(g.config.CheckAPI)
	if mode == "" || (g.apiModel == nil && len(g.config.APIDocs) == 0) {
		return nil
	}
	g.log("📚 Checking API documentation...")
	var findings []finding
	for _, d := range g.apiDiagnostics {
		where := d.File
		if d.Line > 0 {
			where += ":" + fmt.Sprint(d.Line)
		}
		findings = append(findings, finding{file: where, detail: d.Message})
	}
	if g.apiModel != nil {
		for _, f := range apicheck.Check(g.apiModel) {
			findings = append(findings, finding{file: f.Where, detail: f.Message})
		}
	}
	sortFindings(findings)
	return g.report(findings, mode, "API documentation", "API documentation complete",
		"%d API documentation problem(s)")
}
