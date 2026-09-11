package editui

// Editing a site in the browser, over the MCP server that already exists
// (GO-102, phase 1).
//
// The dev server serves the site; the MCP server can already read a document,
// change one passage of it, validate the result and commit it. What was
// missing between them is a client a person can click — and the security
// contract that makes writing files from a web page something other than a
// remote code execution path.
//
// So this package is deliberately thin. It adds no tool, no second
// authorisation layer and no copy of the path checks: every mutation goes
// through mcp.Server.Call, which is the same code the assistant's tool calls
// take. What lives here is the HTTP shape, the token, and the form that knows
// how to render a content schema.
//
// The rules, from the first line rather than later:
//
//   - The endpoints answer only with the mint-on-start token in a HEADER, not a
//     cookie: a page on another origin cannot set one, so a stray browser tab
//     cannot drive this.
//   - Editing runs in the content role. Templates are not editable from a
//     browser; that is work for a person or an agent in the repository.
//   - Every write lands on a git branch of its own, never on the checked-out
//     one, and never without a commit.

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spagu/ssg/internal/mcp"
	"github.com/spagu/ssg/internal/models"
)

// Path is the prefix every editing endpoint lives under.
const Path = "/__edit/"

// TokenHeader carries the session token. A header rather than a cookie, on
// purpose: a cross-origin page cannot add one.
const TokenHeader = "X-SSG-Edit-Token" // #nosec G101 -- a header name, not a credential

// Options configures the editing server.
type Options struct {
	// MCP is the running server every mutation goes through.
	MCP *mcp.Server
	// Token must arrive in TokenHeader on every request.
	Token string
	// Schemas are the per-type frontmatter contracts (#62), which is also the
	// description of the form: a field's declared type chooses its control.
	Schemas map[string]models.ContentSchema
	// Root is the project directory; ContentDirs bound what may be edited. The
	// authoritative check is the MCP server's own, this only shapes messages.
	Root string
	// Branch names the git branch edits land on; empty means one is minted per
	// session.
	Branch string
	// AI answers one editing action, or is nil when the site has no model
	// configured (GO-102 phase 3). It is a function rather than a client so
	// this package does not depend on the AI package, and so the credential
	// stays wherever the caller keeps it.
	AI func(question string, timeout time.Duration) (string, error)
	// ExcerptLimit is the character budget the "shorten" action works to —
	// the same one --check-meta measures against, so the two agree.
	ExcerptLimit int

	// Logf receives one line per refused request.
	Logf func(string, ...any)
}

// Server serves the editing endpoints.
type Server struct {
	opts Options
	mu   sync.Mutex
	// branch is created on the first successful write of a session, so a
	// session that only looks around leaves no trace in git.
	branch string
}

// discardLog is the logger for a server nobody asked to narrate. It exists as
// a named function so an empty body reads as a decision rather than an omission.
func discardLog(string, ...any) {
	// Narration nobody asked for is dropped here rather than at every call site.
}

// New builds an editing server.
func New(opts Options) *Server {
	if opts.Logf == nil {
		opts.Logf = discardLog
	}
	return &Server{opts: opts, branch: opts.Branch}
}

// Handler routes the editing endpoints. Everything under Path requires the
// token; anything else is not ours.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(Path+"doc", s.guard(s.handleDoc))
	mux.HandleFunc(Path+"frontmatter", s.guard(s.handleFrontmatter))
	mux.HandleFunc(Path+"block", s.guard(s.handleBlock))
	mux.HandleFunc(Path+"body", s.guard(s.handleBody))
	mux.HandleFunc(Path+"ai", s.guard(s.handleAI))
	mux.HandleFunc(Path+"status", s.guard(s.handleStatus))
	return mux
}

// Handles reports whether a request path belongs to the editor.
func Handles(path string) bool { return strings.HasPrefix(path, Path) }

// guard enforces the token on every editing endpoint.
func (s *Server) guard(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		given := r.Header.Get(TokenHeader)
		if s.opts.Token == "" || subtle.ConstantTimeCompare([]byte(given), []byte(s.opts.Token)) != 1 {
			s.opts.Logf("   ⚠️  edit: refused %s %s (bad or missing %s)", r.Method, r.URL.Path, TokenHeader)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "this request needs the edit token from the line ssg printed at start-up"})
			return
		}
		next(w, r)
	}
}

// docResponse is what the form is built from.
type docResponse struct {
	Path        string                 `json:"path"`
	Type        string                 `json:"type"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Fields      []field                `json:"fields"`
	Branch      string                 `json:"branch,omitempty"`
}

// field is one control in the form.
type field struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"` // string, int, bool, date, url, list, enum, text
	Values   []string `json:"values,omitempty"`
	Required bool     `json:"required,omitempty"`
	Value    string   `json:"value"`
}

// handleDoc answers with one document's frontmatter and the form for it.
func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
		return
	}
	text, isErr := s.opts.MCP.Call("content_read", map[string]any{"path": rel})
	if isErr {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": text})
		return
	}
	fm, err := readFrontmatter(text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	kind := stringOf(fm["type"])
	if kind == "" {
		kind = "post"
	}
	s.mu.Lock()
	branch := s.branch
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, docResponse{
		Path: rel, Type: kind, Frontmatter: fm,
		Fields: s.fieldsFor(kind, fm), Branch: branch,
	})
}

// fieldsFor builds the form: the type's declared schema when there is one, and
// the keys the document already has when there is not — a site with no
// contracts still gets a form, it just cannot check as much.
func (s *Server) fieldsFor(kind string, fm map[string]interface{}) []field {
	schema, declared := s.opts.Schemas[kind]
	seen := map[string]bool{}
	var out []field

	add := func(name string, rule models.FieldRule) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, field{
			Name: name, Kind: controlFor(name, rule.Type), Values: rule.Values,
			Required: declared && contains(schema.Required, name),
			Value:    stringOf(fm[name]),
		})
	}
	if declared {
		for _, name := range schema.Required {
			add(name, schema.Fields[name])
		}
		for _, name := range sortedKeys(schema.Fields) {
			add(name, schema.Fields[name])
		}
	}
	for _, name := range sortedKeys(fm) {
		add(name, models.FieldRule{})
	}
	return out
}

// controlFor picks the control a field gets. A declared type wins; otherwise
// the name is the only hint available, and two names are worth guessing about
// because getting them wrong makes the form useless for the field people edit
// most.
func controlFor(name, declared string) string {
	if declared != "" {
		return declared
	}
	switch name {
	case "description", "excerpt", "summary":
		return "text"
	case "date", "modified":
		return "date"
	case "tags", "aliases", "categories":
		return "list"
	case "draft", "sticky":
		return "bool"
	}
	return "string"
}

// frontmatterRequest is one field's new value.
type frontmatterRequest struct {
	Path  string `json:"path"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Unset bool   `json:"unset"`
}

// handleFrontmatter writes one frontmatter value.
func (s *Server) handleFrontmatter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req frontmatterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable request: " + err.Error()})
		return
	}
	if req.Path == "" || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path and key are required"})
		return
	}
	current, isErr := s.opts.MCP.Call("content_read", map[string]any{"path": req.Path})
	if isErr {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": current})
		return
	}
	kind := "post"
	if fm, err := readFrontmatter(current); err == nil {
		if t := stringOf(fm["type"]); t != "" {
			kind = t
		}
	}

	var updated string
	var err error
	if req.Unset {
		updated, err = unsetFrontmatter(current, req.Key)
	} else {
		var value interface{}
		if value, err = s.validate(kind, req.Key, req.Value); err == nil {
			updated, err = setFrontmatter(current, req.Key, value)
		}
	}
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	if updated == current {
		writeJSON(w, http.StatusOK, map[string]any{"saved": false, "message": "nothing changed"})
		return
	}
	if text, isErr := s.opts.MCP.Call("content_update", map[string]any{"path": req.Path, "content": updated}); isErr {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": text})
		return
	}
	branch, note := s.commit(req.Path, describe(req))
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "branch": branch, "git": note})
}

// describe summarises an edit for a commit message.
func describe(req frontmatterRequest) string {
	if req.Unset {
		return req.Key + " removed"
	}
	value := req.Value
	if len(value) > 60 {
		value = value[:57] + "…"
	}
	return req.Key + " = " + value
}

// blockResponse is the source a click resolved to.
type blockResponse struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
}

// handleBlock answers with the SOURCE Markdown behind a piece of rendered text.
//
// The editor shows that source, not the rendered HTML: editing a render means a
// lossy round trip back to Markdown, and a paragraph with a link and a bold run
// does not survive it. An author editing `**bold**` is editing what is in the
// file, which is honest and is also the only version that can be saved safely.
func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	clicked := r.URL.Query().Get("text")
	if rel == "" || clicked == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path and text are required"})
		return
	}
	text, isErr := s.opts.MCP.Call("content_read", map[string]any{"path": rel})
	if isErr {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": text})
		return
	}
	doc, err := splitDocument(text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	block, err := FindBlock(doc.body, clicked)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, blockResponse{Path: rel, Source: block.Source, Kind: block.Kind})
}

// bodyRequest is one block being replaced.
type bodyRequest struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// handleBody replaces one block of a page's body.
//
// It goes straight through `content_edit`, which requires the anchor to appear
// exactly once and refuses with the count otherwise. That refusal is the point:
// the failure mode of an editor like this is a green save that changed the
// wrong paragraph, and a refusal an author can act on is strictly better.
func (s *Server) handleBody(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req bodyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable request: " + err.Error()})
		return
	}
	if req.Path == "" || req.Old == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path and old are required"})
		return
	}
	if req.Old == req.New {
		writeJSON(w, http.StatusOK, map[string]any{"saved": false, "message": "nothing changed"})
		return
	}
	text, isErr := s.opts.MCP.Call("content_edit", map[string]any{
		"path": req.Path, "old": req.Old, "new": req.New,
	})
	if isErr {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": text})
		return
	}
	branch, note := s.commit(req.Path, "body edited")
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "branch": branch, "git": note})
}

// handleStatus reports what the editor can do here, so the UI offers only what
// will work rather than buttons that fail.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	branch := s.branch
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"git":    s.opts.MCP.HasTool("git_commit"),
		"branch": branch,
		"ai":     s.opts.AI != nil,
		// The panel renders whatever this lists, so the buttons and the
		// actions cannot disagree about what exists.
		"aiActions": aiActionList(s.opts.ExcerptLimit),
	})
}

// commit puts the change on a branch of its own.
//
// Never the checked-out branch: an edit made by clicking is still a change to
// the repository, and the person who made it should be able to look at it, and
// throw it away, the way they would any other. A project without git is told
// so rather than silently saving into nothing traceable.
func (s *Server) commit(path, what string) (string, string) {
	if !s.opts.MCP.HasTool("git_commit") {
		return "", "saved to the file; this project has no git repository, so there is nothing to commit to"
	}
	s.mu.Lock()
	branch := s.branch
	if branch == "" {
		// The name describes the session; the prefix belongs to the git tool,
		// which already knows what it is. Passing a full name here produced
		// "edit/edit-…".
		name := time.Now().Format("2006-01-02-150405")
		text, isErr := s.opts.MCP.Call("git_new_branch", map[string]any{"name": name})
		if isErr {
			s.mu.Unlock()
			return "", "saved to the file, but the branch could not be created: " + text
		}
		branch = branchFrom(text, name)
		s.branch = branch
	}
	s.mu.Unlock()
	message := fmt.Sprintf("edit: %s: %s", filepath.Base(path), what)
	if text, isErr := s.opts.MCP.Call("git_commit", map[string]any{"message": message}); isErr {
		return branch, "saved to the file, but not committed: " + text
	}
	return branch, "committed to " + branch
}

// branchFrom reads the branch name out of the git tool's reply, falling back
// to the name that was asked for.
func branchFrom(reply, asked string) string {
	for _, f := range strings.Fields(reply) {
		if strings.Contains(f, asked) {
			return strings.Trim(f, "\"'.,")
		}
	}
	return asked
}

// validate checks a value against the type's declared field rule, so a form
// cannot write something the build would reject.
func (s *Server) validate(kind, key, raw string) (interface{}, error) {
	schema, ok := s.opts.Schemas[kind]
	if !ok {
		return typedValue("", raw)
	}
	if contains(schema.Required, key) && strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%s is required for a %s", key, kind)
	}
	rule, declared := schema.Fields[key]
	if !declared {
		return typedValue("", raw)
	}
	if rule.Type == "enum" {
		if !contains(rule.Values, raw) {
			return nil, fmt.Errorf("%s must be one of: %s", key, strings.Join(rule.Values, ", "))
		}
		return raw, nil
	}
	return typedValue(rule.Type, raw)
}

// writeJSON answers with a JSON body.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// contains reports membership.
func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TokenFromEnv returns an operator-supplied token, so a session can be scripted.
func TokenFromEnv() string { return os.Getenv("SSG_EDIT_TOKEN") }
