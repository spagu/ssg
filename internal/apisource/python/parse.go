package python

// stmtKind is the kind of declaration a statement makes.
type stmtKind int

// The declarations the parser keeps.
const (
	sDef       stmtKind = iota // def or async def
	sClass                     // class
	sAssign                    // name = value, name: T = value
	sTypeAlias                 // type X = ... (PEP 695)
)

// decorator is one "@name(args)" line.
type decorator struct {
	name string  // dotted name: "property", "x.setter", "dataclasses.dataclass"
	args []token // the call's arguments; nil when not called
}

// stmt is one declaration, with what pages need of it as raw tokens.
type stmt struct {
	kind       stmtKind
	name       string
	line       int
	decorators []decorator
	async      bool
	typeParams [][]token // PEP 695 type parameters, one token run each
	params     [][]token // def: one token run per parameter
	returns    []token   // def: the "->" annotation
	bases      [][]token // class: one token run per base or keyword
	annot      []token   // assignment annotation
	value      []token   // assignment or type alias value
	augmented  bool      // "name += value"
	doc        string    // docstring, decoded but not dedented
	body       []*stmt   // class body
	overloads  []*stmt   // @overload variants merged into this def
	hasSetter  bool      // a property with a @name.setter
}

// importName is one name a from-import binds.
type importName struct {
	module string // resolved module path ("textkit/lexer"), after relative levels
	name   string // the imported name, "*" for a star import
	alias  string // the local name
	line   int
}

// parsedFile is what the declaration pass reads from one file.
type parsedFile struct {
	doc     string
	stmts   []*stmt
	imports []importName
}

// parser walks the token stream of one file.
type parser struct {
	toks    []token
	i       int
	pkgPath string // the package relative imports start from
	imports []importName
}

// parseFile runs the declaration pass over tokens. pkgPath is the module
// path of the package containing the file, for relative imports.
func parseFile(toks []token, pkgPath string) *parsedFile {
	p := &parser{toks: toks, pkgPath: pkgPath}
	doc, stmts := p.block(false)
	return &parsedFile{doc: doc, stmts: stmts, imports: p.imports}
}

// peek returns the token k positions ahead (EOF past the end).
func (p *parser) peek(k int) token {
	if p.i+k < len(p.toks) {
		return p.toks[p.i+k]
	}
	return token{kind: tEOF}
}

// take consumes and returns the current token (EOF stays put).
func (p *parser) take() token {
	t := p.peek(0)
	if t.kind != tEOF {
		p.i++
	}
	return t
}

// block reads statements up to the DEDENT that closes a nested block, or
// to EOF. It returns the block's docstring and its declarations; a string
// statement right after an assignment documents that assignment.
func (p *parser) block(nested bool) (string, []*stmt) {
	var doc string
	var stmts []*stmt
	first := true
	var last *stmt // the previous statement, when it was a declaration
	for {
		t := p.peek(0)
		switch {
		case t.kind == tEOF:
			return doc, stmts
		case t.kind == tDedent:
			p.i++
			if nested {
				return doc, stmts
			}
		case t.kind == tNewline:
			p.i++
		case t.kind == tIndent:
			p.skipBlock()
		case t.kind == tString && p.stringStatement():
			text := decodeString(t.text)
			if first {
				doc = text
			} else if last != nil && last.kind == sAssign && last.doc == "" {
				last.doc = text
			}
			first, last = false, nil
		default:
			first = false
			if last = p.statement(); last != nil {
				stmts = append(stmts, last)
			}
		}
	}
}

// stringStatement consumes a statement made of one string literal and
// reports whether it was one.
func (p *parser) stringStatement() bool {
	if k := p.peek(1).kind; k != tNewline && k != tEOF {
		return false
	}
	p.i += 2
	return true
}

// statement reads one statement and returns its declaration, or nil.
func (p *parser) statement() *stmt {
	decos := p.decorators()
	t, n := p.peek(0), p.peek(1)
	switch {
	case t.is("def"):
		return p.def(decos, false)
	case t.is("async") && n.is("def"):
		p.i++
		return p.def(decos, true)
	case t.is("class"):
		return p.class(decos)
	case len(decos) > 0:
	case t.is("from"):
		p.fromImport()
		return nil
	case t.is("type") && n.kind == tName && (p.peek(2).is("=") || p.peek(2).is("[")):
		return p.typeStatement()
	case t.kind == tName && !keywords[t.text] && (n.is(":") || n.is("=") || n.is("+=")):
		return p.assignment()
	}
	p.skipStatement()
	return nil
}
