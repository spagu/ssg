package generator

// Recording what a build depended on, and skipping what a change cannot have
// affected (GO-094).
//
// The graph itself lives in internal/depgraph; this is where a build fills it
// in and where it consults a plan. Two rules shape everything here.
//
// **The graph is recorded on every build, incremental or not.** A graph is only
// useful if it describes the build that actually ran, and one written only when
// someone asks for it is one that goes stale the first time they do not.
//
// **A skipped page is a page nothing could have changed.** The plan is built by
// depgraph, which answers "rebuild everything" for every case it is not certain
// about; here we only act on the certainty it hands over.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/models"
)

// GraphDirName is where the dependency graph is kept, under the cache root.
const GraphDirName = "graph"

// startGraph prepares the graph for this build, and loads the previous one so a
// plan can be made against it.
func (g *Generator) startGraph() {
	g.graph = depgraph.New()
	g.prevGraph = depgraph.Load(g.graphDir())
	g.plan = depgraph.Plan{Full: true, Reason: "not an incremental build"}
}

// graphDir is where the graph is persisted, beside the other caches (GO-091).
func (g *Generator) graphDir() string {
	root := g.config.CacheDir
	if root == "" {
		root = ".ssg-cache"
	}
	return filepath.Join(root, GraphDirName)
}

// planIncremental works out what this build can skip, from the previous graph
// and what changed on disk since it was written.
//
// The answer is recorded rather than acted on immediately, because the decision
// has to be visible: an incremental build that quietly became a full one, or
// the reverse, is a build nobody can reason about.
func (g *Generator) planIncremental() {
	if !g.config.Incremental {
		return
	}
	if g.config.Clean {
		g.plan = depgraph.Plan{Full: true, Reason: "--clean empties the output directory, so there is nothing to keep"}
		return
	}
	// Two ways an input can differ from the last build: its bytes changed, or
	// it did not exist then. The second is the one a hash comparison cannot
	// see, because a file the previous graph never recorded is a file it has no
	// hash for — and a build that missed a new page is the silent staleness
	// this whole feature has to avoid.
	changed := append(g.prevGraph.ChangedSince(depgraph.HashFile), g.addedInputs()...)
	sort.Strings(changed)
	g.plan = g.prevGraph.PlanFor(changed)
	if !g.config.Quiet {
		if g.plan.Full {
			fmt.Printf("   🧩 Full build: %s\n", g.plan.Reason)
		} else {
			fmt.Printf("   🧩 Incremental: %s\n", g.plan.Reason)
		}
	}
}

// addedInputs lists the files this build read that the previous one did not.
//
// The plan treats each of them as unknown, which makes the build full — correct
// rather than clever, because a new page can appear in an archive, a feed, a
// tag listing and the sitemap, and none of those edges exist yet.
func (g *Generator) addedInputs() []string {
	if g.graph == nil || g.prevGraph == nil {
		return nil
	}
	var added []string
	for _, id := range g.graph.InputIDs() {
		if !g.prevGraph.Knows(id) {
			added = append(added, id)
		}
	}
	return added
}

// incrementalOutputs is the set of files this build must rewrite, or nil when
// it must write everything.
func (g *Generator) incrementalOutputs() map[string]bool {
	if !g.config.Incremental || g.plan.Full {
		return nil
	}
	out := make(map[string]bool, len(g.plan.Outputs))
	for _, o := range g.plan.Outputs {
		out[o] = true
	}
	return out
}

// skipUnchanged reports whether one page's render can be skipped.
//
// Skipping is only ever right when the previous build wrote this page's output
// AND the plan says nothing changed that reaches it. A page the previous graph
// never saw is rendered, because "I have no record of it" is not the same as
// "it is up to date".
func (g *Generator) skipUnchanged(outputPath string) bool {
	if g.pending == nil {
		return false
	}
	id := g.graphID(outputPath)
	if g.pending[id] {
		return false // this build has to rewrite it
	}
	// Only skip what the previous build actually produced.
	return g.prevGraph.HasOutput(id)
}

// graphID names a file in the graph: relative to the project, with forward
// slashes, so a graph written on one machine reads on another.
func (g *Generator) graphID(path string) string {
	rel := path
	if abs, err := filepath.Abs(path); err == nil {
		if wd, err := filepath.Abs("."); err == nil {
			if r, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(r, "..") {
				rel = r
			}
		}
	}
	return filepath.ToSlash(rel)
}

// recordPageInputs registers what one page's output depends on: its source
// file, and the data files and templates the build read for everyone.
//
// The edges a page does NOT get are as important as the ones it does. A page
// does not depend on other pages here, because a build where it might —
// a template that iterates the whole site — is marked global instead, and a
// global build is never narrowed at all.
func (g *Generator) recordPageOutput(page models.Page, outputPath string) {
	if g.graph == nil {
		return
	}
	id := g.graphID(outputPath)
	g.graph.AddOutput(id)
	if src := pageSourcePath(page); src != "" {
		g.graph.Depends(id, g.graphID(src))
	}
}

// pageSourcePath is the file a page was parsed from, or empty for a page that
// came from somewhere else — a CMS import, a mapped record.
func pageSourcePath(page models.Page) string {
	if page.SourceFile == "" {
		return ""
	}
	if page.SourceDir == "" {
		return page.SourceFile
	}
	return filepath.Join(page.SourceDir, page.SourceFile)
}

// recordInput registers a file the build read.
func (g *Generator) recordInput(path string, kind depgraph.Kind) {
	if g.graph == nil || path == "" {
		return
	}
	hash, ok := depgraph.HashFile(path)
	if !ok {
		// A file that cannot be hashed cannot be compared next time, so the
		// safe reading is that this build cannot be narrowed.
		g.graph.MarkGlobal(fmt.Sprintf("%s could not be read for hashing", path))
		return
	}
	g.graph.AddInput(g.graphID(path), kind, hash)
}

// markGlobalBuild records that this build cannot be narrowed, with the reason a
// person needs in order to change it.
func (g *Generator) markGlobalBuild(reason string) {
	if g.graph != nil {
		g.graph.MarkGlobal(reason)
	}
}

// finishGraph persists the graph this build recorded.
func (g *Generator) finishGraph() {
	if g.graph == nil {
		return
	}
	if err := g.graph.Save(g.graphDir()); err != nil && !g.config.Quiet {
		fmt.Printf("   ⚠️  dependency graph: %v\n", err)
	}
}

// Graph exposes the graph this build recorded, for `ssg graph`.
func (g *Generator) Graph() *depgraph.Graph { return g.graph }

// recordContentInputs registers every file this build read as content, plus the
// templates and data that shape every page.
//
// Templates and the config are recorded as inputs so a change to one is SEEN,
// and their kind makes the plan full — the graph does not model which pages a
// partial reaches, so it refuses to guess.
func (g *Generator) recordContentInputs() {
	if g.graph == nil {
		return
	}
	for _, p := range g.siteData.Pages {
		g.recordInput(pageSourcePath(p), depgraph.KindContent)
	}
	for _, p := range g.siteData.Posts {
		g.recordInput(pageSourcePath(p), depgraph.KindContent)
	}
	g.recordTreeInputs(filepath.Join(g.config.TemplatesDir, g.config.Template), depgraph.KindTemplate)
	g.recordTreeInputs(g.config.DataDir, depgraph.KindData)
	g.recordInput(g.config.ConfigPath, depgraph.KindConfig)

	// A build whose content did not come from files cannot be compared against
	// files next time.
	if g.config.Mddb.Enabled {
		g.markGlobalBuild("content comes from MDDB, which the graph cannot hash")
	}
	if g.config.ExternalSources.Enabled {
		g.markGlobalBuild("external sources can change without a file changing")
	}
	if len(g.cmsImports) > 0 {
		g.markGlobalBuild("a CMS import contributes pages that have no source file")
	}
}

// recordTreeInputs registers every file under a directory.
func (g *Generator) recordTreeInputs(dir string, kind depgraph.Kind) {
	if dir == "" {
		return
	}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is not a build failure
		}
		g.recordInput(path, kind)
		return nil
	})
}
