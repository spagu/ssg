package main

// `ssg repair` finds source Markdown that renders as literal text instead of as
// markup and, with --fix, rewrites it in place.
//
// The case it was built for: a migrated WordPress page whose builder markup is
// tab-indented. CommonMark reads four columns of indentation as a code block, so
// the visitor gets `</div>` in monospace down the middle of the page and the
// build reports nothing wrong — it did exactly what the source said (#127).
//
// Dry run by default. A repair rewrites the author's files, so it never happens
// as a side effect of asking what is wrong; the report exits non-zero so CI can
// gate on it.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/repair"
)

type repairFlags struct {
	fix   bool
	quiet bool
	paths []string
}

// repairResult is one scanned file's outcome.
type repairResult struct {
	path     string
	findings []repair.Finding
	fixed    int
}

func runRepair(args []string) int {
	flags, code := parseRepairFlags(args)
	if code >= 0 {
		return code
	}

	roots := flags.paths
	if len(roots) == 0 {
		roots = []string{repairDefaultRoot()}
	}

	var results []repairResult
	for _, root := range roots {
		found, err := repairScanRoot(root, flags.fix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			return 1
		}
		results = append(results, found...)
	}
	return reportRepair(results, flags)
}

// repairDefaultRoot is the project's content directory, so a bare `ssg repair`
// in a project root does the obvious thing.
func repairDefaultRoot() string {
	cfg, err := loadConfigFile(config.FindConfigFile())
	if err != nil || cfg == nil || cfg.ContentDir == "" {
		return "content"
	}
	return cfg.ContentDir
}

// repairScanRoot scans one file or directory tree, repairing as it goes when
// fix is set. Only files with findings come back.
func repairScanRoot(root string, fix bool) ([]repairResult, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", root, err)
	}
	if !info.IsDir() {
		res, scanErr := repairFile(root, fix)
		if scanErr != nil || len(res.findings) == 0 {
			return nil, scanErr
		}
		return []repairResult{res}, nil
	}

	var out []repairResult
	walkErr := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return err
		}
		res, scanErr := repairFile(path, fix)
		if scanErr != nil {
			return scanErr
		}
		if len(res.findings) > 0 {
			out = append(out, res)
		}
		return nil
	})
	return out, walkErr
}

// repairFile scans one Markdown file, rewriting it only when fix is set AND the
// content actually changes — an untouched file keeps its mtime, so a watch loop
// does not rebuild over a no-op.
func repairFile(path string, fix bool) (repairResult, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from the operator's own content tree
	if err != nil {
		return repairResult{path: path}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	res := repairResult{path: path, findings: repair.Scan(string(data))}
	if !fix || len(res.findings) == 0 {
		return res, nil
	}

	repaired, changed := repair.Apply(string(data))
	if changed == 0 {
		return res, nil
	}
	// Preserve the file's own permissions rather than imposing new ones.
	mode := os.FileMode(0o600)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	// #nosec G703 G304 -- path comes from the operator's own content tree (a CLI
	// argument or the configured content_dir), and the write is the point of --fix.
	if err := os.WriteFile(path, []byte(repaired), mode); err != nil {
		return res, fmt.Errorf("cannot write %s: %w", path, err)
	}
	res.fixed = changed
	return res, nil
}

// reportRepair prints what was found or fixed and picks the exit code: a dry
// run that found something exits 1 so CI can gate on it, while a successful
// --fix exits 0 — the problem is gone.
func reportRepair(results []repairResult, flags repairFlags) int {
	if len(results) == 0 {
		if !flags.quiet {
			fmt.Println("✅ No indented markup found — every page renders as markup.")
		}
		return 0
	}

	blocks, lines := 0, 0
	for _, r := range results {
		blocks += len(r.findings)
		lines += r.fixed
		if flags.quiet {
			continue
		}
		fmt.Printf("\n📄 %s\n", r.path)
		for _, f := range r.findings {
			fmt.Printf("   line %d: %d line(s) of markup indented as code → %s\n", f.Line, f.Lines, f.Sample)
		}
	}

	if flags.fix {
		fmt.Printf("\n🔧 Repaired %d block(s) across %d file(s) (%d lines dedented).\n",
			blocks, len(results), lines)
		fmt.Println("   Rebuild to see it: ssg --config .ssg.yaml")
		return 0
	}
	fmt.Printf("\n⚠️  %d block(s) of markup across %d file(s) render as literal text.\n", blocks, len(results))
	fmt.Println("   Fix them in place with: ssg repair --fix")
	fmt.Println("   The usual cause is an export that indented its markup (page builders do);")
	fmt.Println("   re-exporting with wpexporter 1.8.2+ produces clean sources.")
	return 1
}

func parseRepairFlags(args []string) (repairFlags, int) {
	var f repairFlags
	for _, arg := range args {
		switch {
		case arg == "--fix":
			f.fix = true
		case arg == "--quiet" || arg == "-q":
			f.quiet = true
		case arg == "--help" || arg == "-h":
			printRepairUsage()
			return f, 0
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(os.Stderr, "❌ unknown flag %q\n\n", arg)
			printRepairUsage()
			return f, 2
		default:
			f.paths = append(f.paths, arg)
		}
	}
	return f, -1
}

func printRepairUsage() {
	fmt.Print(`usage: ssg repair [path...] [--fix]

   ssg repair                     report indented markup in the content dir
   ssg repair --fix               rewrite the affected files in place
   ssg repair content/site/pages  scan one directory or file

Finds Markdown whose markup is indented four columns or more, which CommonMark
renders as a literal code block — the shape a page-builder export leaves behind.
Front matter, fenced code blocks and list continuations are never touched.

flags:
   --fix        apply the repairs (default: report only)
   --quiet, -q  print the summary only

exit codes: 0 nothing to repair (or --fix succeeded), 1 findings in a dry run
`)
}
