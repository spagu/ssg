package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apisource"
	"github.com/tdewolff/parse/v2/js"
)

// cjsWarning is reported once per file that assigns CommonJS exports.
const cjsWarning = "CommonJS exports are read on a best-effort basis"

// collectCommonJS reads the CommonJS export forms: "module.exports = …"
// (an object literal lists the exports, anything else is the default) and
// "exports.x = …" / "module.exports.x = …". require() is not followed.
func (f *file) collectCommonJS(ld *loader, n *js.ExprStmt) {
	assign, ok := n.Value.(*js.BinaryExpr)
	if !ok || assign.Op != js.EqToken {
		return
	}
	lhs := nodeText(assign.X)
	switch {
	case lhs == "module.exports":
		f.markCommonJS(ld, "module.exports")
		f.cjsModule(assign.Y)
	case strings.HasPrefix(lhs, "exports.") || strings.HasPrefix(lhs, "module.exports."):
		name := lhs[strings.LastIndex(lhs, ".")+1:]
		if isIdent(name) && strings.Count(strings.TrimPrefix(lhs, "module."), ".") == 1 {
			f.markCommonJS(ld, "exports."+name)
			f.cjsExport(name, assign.Y, f.siteOf("exports."+name))
		}
	}
}

// markCommonJS warns, once per file, that its exports are read loosely.
func (f *file) markCommonJS(ld *loader, key string) {
	if f.cjs {
		return
	}
	f.cjs = true
	ld.diag(apisource.Warning, f.path, f.scan.tokenLine(f.siteOf(key)), cjsWarning)
}

// cjsModule reads "module.exports = value".
func (f *file) cjsModule(value js.IExpr) {
	obj, ok := value.(*js.ObjectExpr)
	if !ok {
		f.cjsExport("default", value, f.siteOf("module.exports"))
		return
	}
	cur := f.scan.membersAfter(f.siteOf("module.exports"), "")
	for _, p := range obj.List {
		name := propertyName(p)
		if name == "" {
			continue
		}
		f.cjsExport(name, p.Value, cur.find(name))
	}
}

// cjsExport exports a value under a name: a plain identifier refers to the
// local declaration, anything else is declared where it is written.
func (f *file) cjsExport(name string, value js.IExpr, start int) {
	if v, ok := value.(*js.Var); ok {
		f.exports[name] = exportRef{name: string(v.Name())}
		return
	}
	local := "exports." + name
	f.declareAt(local, name, value, false, start)
	f.exports[name] = exportRef{name: local}
}

// propertyName is the key of an object literal property, or "" for a
// spread or computed key.
func propertyName(p js.Property) string {
	if m, ok := p.Value.(*js.MethodDecl); ok && p.Name == nil {
		if m.Name.Private != nil || m.Name.IsComputed() {
			return ""
		}
		return string(m.Name.Literal.Data)
	}
	if p.Spread || p.Name == nil || p.Name.IsComputed() {
		return ""
	}
	return string(p.Name.Literal.Data)
}

// siteOf returns the start token of a site, or -1.
func (f *file) siteOf(key string) int {
	if i, ok := f.sites[key]; ok {
		return i
	}
	return -1
}
