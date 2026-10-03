package js

// Sites are where top-level declarations start, keyed by what they declare:
//
//	"name"            function, class, const/let/var name
//	"default"         export default …
//	"module.exports"  module.exports = …
//	"exports.name"    exports.name = … and module.exports.name = …
//
// Top-level names are unique within a module, so the first match is the
// declaration. The start is the first token of the statement, "export" and
// "async" included, which is where its doc comment sits.

// statementPrefix are the tokens that may precede a declaration keyword in
// the same statement.
var statementPrefix = map[string]bool{"export": true, "default": true, "async": true}

// varEnders end the run of comma-separated names of a const/let/var.
var varEnders = map[string]bool{";": true, "export": true, "import": true, "function": true, "class": true}

// siteIndex finds the declaration sites of a file.
type siteIndex struct {
	s     *scan
	sites map[string]int
	inVar int // start of the const/let/var statement being read, or -1
}

// topLevelSites indexes the top-level declarations of a scanned file.
func (s *scan) topLevelSites() map[string]int {
	ix := &siteIndex{s: s, sites: map[string]int{}, inVar: -1}
	for i, t := range s.toks {
		if t.depth == 0 {
			ix.visit(i)
		}
	}
	return ix.sites
}

// visit looks at one depth-0 token.
func (ix *siteIndex) visit(i int) {
	s := ix.s
	t := s.toks[i].text
	if varEnders[t] {
		ix.inVar = -1
	}
	switch t {
	case "function":
		j := i + 1
		if s.text(j) == "*" {
			j++
		}
		ix.addIdent(j, i)
	case "class":
		ix.addIdent(i+1, i)
	case "const", "let", "var":
		ix.inVar = i
		ix.addIdent(i+1, i)
	case ",":
		if ix.inVar >= 0 {
			ix.addIdent(i+1, i+1) // the statement's comment belongs to its first name
		}
	case "default":
		if s.text(i-1) == "export" {
			ix.add("default", i)
		}
	case "module", "exports":
		ix.visitCommonJS(i)
	}
}

// visitCommonJS indexes module.exports = …, module.exports.x = … and
// exports.x = ….
func (ix *siteIndex) visitCommonJS(i int) {
	s := ix.s
	if s.text(i-1) == "." {
		return
	}
	j := i
	if s.text(i) == "module" {
		if s.text(i+1) != "." || s.text(i+2) != "exports" {
			return
		}
		j = i + 2
		if s.text(j+1) == "=" {
			ix.add("module.exports", i)
			return
		}
	}
	if s.text(j+1) == "." && isIdent(s.text(j+2)) && s.text(j+3) == "=" {
		ix.add("exports."+s.text(j+2), i)
	}
}

// addIdent records token j as a declared name when it is an identifier.
func (ix *siteIndex) addIdent(j, at int) {
	if name := ix.s.text(j); isIdent(name) && name != "extends" {
		ix.add(name, at)
	}
}

// add records a site unless the key is already known.
func (ix *siteIndex) add(key string, at int) {
	if _, ok := ix.sites[key]; ok {
		return
	}
	ix.sites[key] = ix.s.statementStart(at)
}

// statementStart walks back from token i over export/default/async at the
// same depth.
func (s *scan) statementStart(i int) int {
	for i > 0 && statementPrefix[s.toks[i-1].text] && s.toks[i-1].depth == s.toks[i].depth {
		i--
	}
	return i
}

// isIdent reports whether text looks like an identifier.
func isIdent(text string) bool {
	if text == "" {
		return false
	}
	for i, r := range text {
		letter := r == '_' || r == '$' || r >= 0x80 || (r|0x20 >= 'a' && r|0x20 <= 'z')
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}
