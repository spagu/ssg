package mcp

// The API section (GO-111): questions about the code a site documents,
// answered from the api.json the last build wrote. An agent asking "what does
// parse take?" gets the signature, not a page of HTML or the whole model.
// Read-only, like the site section.

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spagu/ssg/internal/apimodel"
)

// apiFile is the generator's published model.
const apiFile = "api.json"

// apiCache keeps the decoded model until api.json changes.
type apiCache struct {
	mu    sync.Mutex
	mtime time.Time
	api   *apimodel.API
	ix    *apimodel.Index
}

// apiTools is the section, present with the site tools.
func (s *Server) apiTools() []tool {
	return []tool{
		{
			name: "api_search",
			description: "API · Find documented code symbols by name or summary text: returns id, kind, " +
				"name and summary. Filter by `kind` (class, function, method, type…); `limit` (default 20). " +
				"Needs api_docs in the config and a completed build.",
			schema: objectSchema(map[string]any{
				"query": stringProp("Name or words to look for"),
				"kind":  stringProp("Optional symbol kind"),
				"limit": intProp("Maximum results (default 20, max 200)"),
			}, "query"),
			handler: s.apiSearch,
		},
		{
			name: "api_symbol",
			description: "API · One symbol by `id` (e.g. \"core/src/lex#Lexer\"): signatures, parameters, " +
				"types, documentation and source. Members are listed by id and summary — ask for one by its id.",
			schema:  objectSchema(map[string]any{"id": stringProp("Symbol or member id")}, "id"),
			handler: s.apiSymbol,
		},
		{
			name:        "api_module",
			description: "API · A module's exports by `id` (e.g. \"core/src/lex\"): id, kind, name, summary of each.",
			schema:      objectSchema(map[string]any{"id": stringProp("Module id")}, "id"),
			handler:     s.apiModule,
		},
	}
}

// loadAPI reads api.json, reusing the decoded model while the file is unchanged.
func (s *Server) loadAPI() (*apimodel.API, *apimodel.Index, error) {
	if s.opts.OutputDir == "" {
		return nil, nil, errors.New("the server was started without an output directory, so there is no api.json to read")
	}
	path := filepath.Join(s.opts.OutputDir, apiFile)
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, errors.New("no api.json in the output: add api_docs to the config and build")
	}
	s.apis.mu.Lock()
	defer s.apis.mu.Unlock()
	if s.apis.api != nil && info.ModTime().Equal(s.apis.mtime) {
		return s.apis.api, s.apis.ix, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the build's own output file
	if err != nil {
		return nil, nil, err
	}
	api, err := apimodel.Decode(data)
	if err != nil {
		return nil, nil, err
	}
	s.apis.api, s.apis.ix, s.apis.mtime = api, apimodel.NewIndex(api), info.ModTime()
	return api, s.apis.ix, nil
}

// apiEntry is one symbol in a list: enough to choose, not to read.
type apiEntry struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

func entryOf(sym *apimodel.Symbol) apiEntry {
	e := apiEntry{ID: sym.ID, Kind: string(sym.Kind), Name: sym.Name}
	if sym.Doc != nil {
		e.Summary = sym.Doc.Summary
	}
	return e
}

// apiSearch matches the query against names first, then summaries.
func (s *Server) apiSearch(args map[string]any) toolResult {
	api, _, err := s.loadAPI()
	if err != nil {
		return errResult(err.Error())
	}
	query, _ := strArg(args, "query")
	query = strings.ToLower(strings.TrimSpace(query))
	kind, _ := strArg(args, "kind")
	limit := min(max(intArg(args, "limit", 20), 1), 200)
	var byName, byText []apiEntry
	for _, p := range api.Packages {
		for _, m := range p.Modules {
			apimodel.Walk(m.Symbols, func(sym, _ *apimodel.Symbol) {
				if kind != "" && string(sym.Kind) != kind {
					return
				}
				e := entryOf(sym)
				switch {
				case strings.Contains(strings.ToLower(sym.Name), query):
					byName = append(byName, e)
				case strings.Contains(strings.ToLower(e.Summary), query):
					byText = append(byText, e)
				}
			})
		}
	}
	sort.SliceStable(byName, func(i, j int) bool { return len(byName[i].Name) < len(byName[j].Name) })
	out := append(byName, byText...)
	if len(out) > limit {
		out = out[:limit]
	}
	return jsonResult(map[string]any{"results": out, "count": len(out)})
}

// apiSymbol returns one symbol, its members trimmed to entries.
func (s *Server) apiSymbol(args map[string]any) toolResult {
	_, ix, err := s.loadAPI()
	if err != nil {
		return errResult(err.Error())
	}
	id, _ := strArg(args, "id")
	sym, ok := ix.Symbol(id)
	if !ok {
		return errResult("no documented symbol " + id + " — find ids with api_search")
	}
	view := *sym
	members := make([]apiEntry, 0, len(sym.Members))
	for _, m := range sym.Members {
		members = append(members, entryOf(m))
	}
	view.Members = nil
	return jsonResult(map[string]any{"symbol": view, "members": members, "parent": ix.Parent(id)})
}

// apiModule lists a module's exports.
func (s *Server) apiModule(args map[string]any) toolResult {
	_, ix, err := s.loadAPI()
	if err != nil {
		return errResult(err.Error())
	}
	id, _ := strArg(args, "id")
	m, ok := ix.Module(id)
	if !ok {
		return errResult("no documented module " + id)
	}
	exports := make([]apiEntry, 0, len(m.Symbols))
	for _, sym := range m.Symbols {
		exports = append(exports, entryOf(sym))
	}
	return jsonResult(map[string]any{"module": m.ID, "path": m.Path, "doc": m.Doc, "exports": exports})
}
