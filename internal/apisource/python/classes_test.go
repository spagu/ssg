package python

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// checkTwo checks the members of class Two in parseSrc.
func checkTwo(t *testing.T, two *apimodel.Symbol) {
	t.Helper()
	if got := names(two.Members); !equal(got, []string{"__init__", "PLAIN", "count", "size", "cached", "make", "args_first", "gone"}) {
		t.Errorf("Two members = %v", got)
	}
	if !two.Flags.Abstract || len(two.Extends) != 1 {
		t.Errorf("Two flags/extends = %+v %v", two.Flags, two.Extends)
	}
	ctor := two.Members[0]
	if ctor.Signatures[0].Params[0].Doc != "The n." {
		t.Errorf("ctor param doc from class docstring = %q", ctor.Signatures[0].Params[0].Doc)
	}
	count := two.Members[2]
	if !count.Flags.Static || count.Type.String() != "int" || count.Doc == nil {
		t.Errorf("count = %+v %s %+v", count.Flags, count.Type, count.Doc)
	}
	size := two.Members[3]
	if !size.Flags.Readonly || size.Type.String() != "int" {
		t.Errorf("size = %+v %s", size.Flags, size.Type)
	}
	if mk := two.Members[5]; len(mk.Signatures) != 2 || !mk.Flags.Static || len(mk.Signatures[0].Params) != 1 {
		t.Errorf("make = %+v", mk)
	}
	if af := two.Members[6]; len(af.Signatures[0].Params) != 1 || !af.Signatures[0].Params[0].Rest {
		t.Errorf("args_first keeps *args: %+v", af.Signatures[0].Params)
	}
	if gone := two.Members[7]; !gone.Flags.Abstract || gone.Doc == nil || gone.Doc.Deprecated == nil {
		t.Errorf("gone = %+v", gone)
	}
}

// checkDataclass checks the generated and the explicit dataclass __init__.
func checkDataclass(t *testing.T, dc, withInit *apimodel.Symbol) {
	t.Helper()
	ctor := dc.Members[0]
	sig := ctor.Signatures[0]
	if sig.Code != `def __init__(self, a: int, *, b: InitVar[str] = field(default="x"), c: int = field(default_factory=int), d: list = field()) -> None` {
		t.Errorf("DC init = %q", sig.Code)
	}
	opt := []bool{false, true, true, false}
	for i, p := range sig.Params {
		if p.Optional != opt[i] {
			t.Errorf("DC param %s optional = %v", p.Name, p.Optional)
		}
	}
	if sig.Params[1].Type.String() != "str" || sig.Params[0].Doc != "The a." {
		t.Errorf("DC params = %+v %+v", sig.Params[0], sig.Params[1])
	}
	if a := dc.Members[1]; a.Doc == nil || a.Doc.Summary != "The a." || a.Flags.Readonly {
		t.Errorf("DC.a = %+v", a)
	}
	if ctor := withInit.Members[0]; len(ctor.Signatures[0].Params) != 0 {
		t.Errorf("explicit __init__ replaced: %+v", ctor.Signatures[0])
	}
}
