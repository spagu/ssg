package dts

import (
	"os"
	"path"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// HasDeclarations reports whether the package at root ships declarations:
// package.json "types"/"typings", an "exports" condition "types", or an
// index.d.ts next to the entry. The generator uses it to pick this extractor.
func HasDeclarations(root string) bool {
	r, err := os.OpenRoot(root)
	if err != nil {
		return false
	}
	defer func() { _ = r.Close() }()
	pj, _ := readPackageJSON(r)
	// A declaration package.json names but the build has not written yet
	// (types: dist/index.d.ts before `npm run build`) does not count: reading
	// declarations that do not exist documents nothing, and the package's
	// JavaScript is still there to read.
	for _, p := range manifestEntries(pj) {
		if findFile(r, declarationCandidates(p)) != "" {
			return true
		}
	}
	return findFile(r, fallbackEntries(pj)) != ""
}

// fallbackEntries are the files tried when package.json names no
// declarations: the declaration file beside "main", then index.d.ts.
func fallbackEntries(pj packageJSON) []string {
	var out []string
	if main := cleanRel(pj.Main); main != "" {
		out = append(out, declarationCandidates(main)...)
	}
	return append(out, "index.d.ts")
}

// discoverEntries returns the declaration entries of a package, relative to
// the root: the configured ones, else those package.json names, else a
// fallback file. A configured entry with no declaration file is reported.
func discoverEntries(r *os.Root, cfg apisource.Config, pj packageJSON) ([]string, []apisource.Diagnostic) {
	if len(cfg.Entries) == 0 {
		if list := manifestEntries(pj); len(list) > 0 {
			for i, p := range list {
				list[i] = firstNonEmpty(findFile(r, declarationCandidates(p)), p)
			}
			return list, nil
		}
		if f := findFile(r, fallbackEntries(pj)); f != "" {
			return []string{f}, nil
		}
		return nil, nil
	}
	var out []string
	var diags []apisource.Diagnostic
	for _, e := range cfg.Entries {
		rel := cleanRel(e)
		f := ""
		if rel != "" {
			f = findFile(r, declarationCandidates(rel))
		}
		if f == "" {
			diags = append(diags, apisource.Diagnostic{Severity: apisource.Error, File: e, Message: "entry has no declaration file"})
			continue
		}
		out = append(out, f)
	}
	return out, diags
}

// findFile returns the first candidate that is a regular file under r.
func findFile(r *os.Root, candidates []string) string {
	for _, c := range candidates {
		if info, err := r.Stat(c); err == nil && info.Mode().IsRegular() {
			return c
		}
	}
	return ""
}

// declarationExts pairs JavaScript and TypeScript source extensions with the
// declaration extension that describes them.
var declarationExts = [][2]string{
	{".mjs", ".d.mts"}, {".cjs", ".d.cts"}, {".js", ".d.ts"}, {".jsx", ".d.ts"},
	{".mts", ".d.mts"}, {".cts", ".d.cts"}, {".tsx", ".d.ts"}, {".ts", ".d.ts"},
}

// declarationCandidates lists the declaration files p may mean: p itself
// when it is one, the declaration of a source file, or p with a declaration
// extension or an index file.
func declarationCandidates(p string) []string {
	if isDeclaration(p) {
		return []string{p}
	}
	for _, pair := range declarationExts {
		if strings.HasSuffix(p, pair[0]) {
			return []string{strings.TrimSuffix(p, pair[0]) + pair[1]}
		}
	}
	return []string{p + ".d.ts", p + ".d.mts", p + ".d.cts", p + "/index.d.ts"}
}

// isDeclaration reports whether p names a declaration file.
func isDeclaration(p string) bool {
	return strings.HasSuffix(p, ".d.ts") || strings.HasSuffix(p, ".d.mts") || strings.HasSuffix(p, ".d.cts")
}

// relativeTarget resolves a relative module specifier written in the file
// at from to a path under the root; "" for a bare specifier (another
// package) or one that leaves the root.
func relativeTarget(from, spec string) string {
	if spec != "." && spec != ".." && !strings.HasPrefix(spec, "./") && !strings.HasPrefix(spec, "../") {
		return ""
	}
	return cleanRel(path.Join(path.Dir(from), spec))
}
