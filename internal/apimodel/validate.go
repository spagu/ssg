package apimodel

import (
	"fmt"
	"sort"
	"strings"
)

// Problem is one thing wrong with a model, located by ID.
type Problem struct {
	ID      string
	Message string
}

func (p Problem) String() string { return p.ID + ": " + p.Message }

// Validate reports what would make pages or links wrong: an empty or
// duplicated ID, a symbol whose ID is not under its module, a member whose ID
// is not under its owner, a kind the model does not know, and a type
// reference to a symbol that does not exist. Every problem is reported, in ID
// order, so one run shows the whole list.
func Validate(a *API) []Problem {
	var probs []Problem
	seen := map[string]bool{}
	add := func(id, format string, args ...any) {
		probs = append(probs, Problem{ID: id, Message: fmt.Sprintf(format, args...)})
	}
	claim := func(id string) {
		if id == "" {
			add("(empty)", "an ID is empty")
			return
		}
		if seen[id] {
			add(id, "the ID is used twice")
		}
		seen[id] = true
	}
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			claim(m.ID)
			Walk(m.Symbols, func(s, parent *Symbol) {
				claim(s.ID)
				owner := m.ID + "#"
				if parent != nil {
					owner = parent.ID + "."
				}
				if s.ID != "" && !strings.HasPrefix(s.ID, owner) {
					add(s.ID, "is not under %q", strings.TrimRight(owner, "#."))
				}
				if !knownKinds[s.Kind] {
					add(s.ID, "unknown kind %q", s.Kind)
				}
			})
		}
	}
	ix := NewIndex(a)
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			Walk(m.Symbols, func(s, _ *Symbol) {
				for _, ref := range symbolRefs(s) {
					if _, ok := ix.Symbol(ref); !ok {
						add(s.ID, "refers to %q, which is not documented", ref)
					}
				}
			})
		}
	}
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].ID < probs[j].ID })
	return probs
}

// knownKinds is the set Validate accepts.
var knownKinds = map[Kind]bool{
	KindNamespace: true, KindClass: true, KindInterface: true, KindFunction: true,
	KindMethod: true, KindConstructor: true, KindProperty: true, KindAccessor: true,
	KindType: true, KindEnum: true, KindEnumMember: true, KindVariable: true,
}

// symbolRefs collects the Ref of every TypeRef a symbol's own declaration uses.
func symbolRefs(s *Symbol) []string {
	var refs []string
	var visit func(t *TypeRef)
	visitSig := func(sig *Signature) {
		if sig == nil {
			return
		}
		for _, tp := range sig.TypeParams {
			visit(tp.Constraint)
			visit(tp.Default)
		}
		for _, p := range sig.Params {
			visit(p.Type)
		}
		visit(sig.Returns)
	}
	visit = func(t *TypeRef) {
		if t == nil {
			return
		}
		if t.Ref != "" {
			refs = append(refs, t.Ref)
		}
		for _, a := range t.Args {
			visit(a)
		}
		for _, f := range t.Fields {
			visit(f.Type)
		}
		visitSig(t.Signature)
	}
	visit(s.Type)
	for _, t := range append(append([]*TypeRef{}, s.Extends...), s.Implements...) {
		visit(t)
	}
	for _, tp := range s.TypeParams {
		visit(tp.Constraint)
		visit(tp.Default)
	}
	for _, sig := range s.Signatures {
		visitSig(sig)
	}
	return refs
}
