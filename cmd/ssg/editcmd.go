package main

// `ssg --http --watch --edit` (GO-102, phase 1).
//
// The dev server already serves the site and pushes a reload after every
// rebuild; the MCP server already reads a document, changes one passage of it,
// validates the result and commits it. This wires the two together and puts a
// form in front of them.
//
// The safety rules come first because this writes files and runs git from a
// web page. MCP learned that lesson already, and its answers are copied rather
// than re-derived: a token in a header, loopback unless told otherwise, one
// place that decides what a path may be.

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/editui"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/mcp"
)

// editSession is what a running editor needs, published as one value so a
// handler built on another goroutine sees a consistent pair.
type editSession struct {
	server *editui.Server
	script string
}

// currentEdit holds the running session, or nil. Process-wide for the same
// reason the live-reload hub is: the command that starts it and the handler
// that reads it are on different goroutines.
var currentEdit atomic.Pointer[editSession]

// checkEditPreconditions refuses the combinations that would make edit mode
// unsafe, and returns the reason.
//
//   - Without --watch there is no rebuild, so a save would change the file and
//     leave the browser showing the old page: an editor that appears not to work.
//   - Off loopback without a token, this is a file-writing endpoint on a
//     network. It does not start.
func checkEditPreconditions(cfg *config.Config, token string) error {
	if !cfg.HTTP {
		return fmt.Errorf("--edit needs the preview server: add --http")
	}
	if !cfg.Watch {
		return fmt.Errorf("--edit needs --watch, or a saved change would never reach the page")
	}
	host := cfg.Host
	if host != "" && !mcp.IsLoopback(host) && token == "" {
		return fmt.Errorf("--edit on %s would expose a file-writing endpoint to the network; "+
			"set SSG_EDIT_TOKEN to a secret you choose, or listen on localhost", host)
	}
	return nil
}

// startEditMode prepares the editor for a run, returning false when the run
// should stop. It is a no-op unless --edit was given.
func startEditMode(genCfg generator.Config, cfg *config.Config) bool {
	if !cfg.Edit {
		return true
	}
	token := editui.TokenFromEnv()
	if err := checkEditPreconditions(cfg, token); err != nil {
		errf("❌ %v\n", err)
		return false
	}
	if token == "" {
		minted, err := mcp.NewToken()
		if err != nil {
			errf("❌ --edit: could not mint a session token: %v\n", err)
			return false
		}
		token = minted
	}

	// The content role, and only it: editing a template from a browser is not
	// part of this, and the role boundary that says so already exists.
	server := mcp.NewServer(mcp.Options{
		Root:         ".",
		TemplateDirs: []string{cfg.TemplatesDir},
		StaticDirs:   []string{cfg.StaticDir},
		MediaRoots:   mediaRootsOf(cfg),
		ContentDirs:  contentRoots(cfg),
		OutputDir:    cfg.OutputDir,
		Roles:        map[string]bool{"content": true},
		Version:      Version,
		// Local git: a save lands on its own branch even without a forge
		// token. Opening a pull request still needs one (GO-102).
		Git:     localGit(buildMCPGit(cfg), contentRoots(cfg)),
		Logf:    func(format string, a ...any) { errf(format+"\n", a...) },
		Rebuild: newMCPRebuilder(genCfg, cfg).rebuild,
	})
	currentEdit.Store(&editSession{
		server: editui.New(editui.Options{
			MCP:     server,
			Token:   token,
			Schemas: cfg.ContentSchemas,
			Root:    ".",
			Logf:    func(format string, a ...any) { errf(format+"\n", a...) },
		}),
		script: editui.Script(token),
	})
	if !cfg.Quiet {
		fmt.Println("✎  Edit mode is on. Click a marked region to edit its frontmatter.")
		fmt.Printf("   Token: %s   (set SSG_EDIT_TOKEN to choose your own)\n", token)
		fmt.Println("   Saves land on a git branch of their own, never on the checked-out one.")
	}
	return true
}

// editMiddleware serves the editing endpoints and injects the panel into every
// page. A no-op unless --edit started an editor.
func editMiddleware(next http.Handler) http.Handler {
	session := currentEdit.Load()
	if session == nil {
		return next
	}
	endpoints := session.server.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if editui.Handles(r.URL.Path) {
			endpoints.ServeHTTP(w, r)
			return
		}
		rec := &editInjectWriter{ResponseWriter: w, script: session.script}
		next.ServeHTTP(rec, r)
		rec.finish()
	})
}

// editInjectWriter buffers a response so the panel can be inserted into HTML.
// Non-HTML and already-encoded responses pass through untouched, exactly as the
// live-reload injector treats them.
type editInjectWriter struct {
	http.ResponseWriter
	script string
	status int
	body   strings.Builder
}

func (w *editInjectWriter) WriteHeader(code int) { w.status = code }

func (w *editInjectWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

func (w *editInjectWriter) finish() {
	body := w.body.String()
	ct := w.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "text/html") && w.Header().Get("Content-Encoding") == "" {
		if i := strings.LastIndex(body, "</body>"); i >= 0 {
			body = body[:i] + w.script + body[i:]
		} else {
			body += w.script
		}
		w.Header().Del("Content-Length")
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write([]byte(body))
}

// editModeEnabled reports whether this process is serving an editor.
func editModeEnabled() bool { return currentEdit.Load() != nil }

// stopEditMode clears the running session.
func stopEditMode() { currentEdit.Store(nil) }

// localGit turns on the local half of the git flow for an editing session.
// Only the content directories are staged. The editor writes content and
// nothing else, and a project whose templates live outside the repository — a
// shared theme directory, say — would otherwise fail every commit with
// "outside repository" for a directory the edit never touched.
func localGit(g mcp.GitOptions, contentDirs []string) mcp.GitOptions {
	g.Local = true
	g.StageDirs = contentDirs
	if g.BranchPrefix == "" {
		g.BranchPrefix = "edit/"
	}
	return g
}
