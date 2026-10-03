package python

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// parseSrc is a module exercising the statements the parser keeps or skips.
const parseSrc = `"""Module doc."""
import os
if os.name == "nt":
    def hidden(): pass
    class Hidden: pass
else:
    pass
try:
    from x import y
except ImportError:
    y = None
for i in range(3): pass
x, y2 = 1, 2
self_x = obj.attr = 3
os.environ["A"] = "b"
print("doc?")
"""Not a docstring: follows a call."""
A = 1; B = 2
LONG = [` + "\n" + `    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
]
FINAL: Final[int] = 3
counter: int
"""A counter."""
def one(): "Doc on one line."; return 1
async def two(a, /, b: int = 1, *, c, **kw) -> "Two": ...
@decorator
x2 = 1
@deco.with_args(1, key="v")
def decorated(): pass
def (bad): pass
class (Bad): pass
class Plain: "Doc."
class Two(Generic[T], metaclass=ABCMeta):
    """Two.

    Args:
        n: The n.
    """
    PLAIN = 1
    _private = 2
    count: ClassVar[int] = 0
    "Count doc."
    items += [1]
    def __init__(self, n: int): ...
    @overload
    @staticmethod
    def make(n: int) -> int: ...
    @overload
    @staticmethod
    def make(n: str) -> str: ...
    @property
    def size(self):
        """Size.

        :rtype: int
        """
    @size.deleter
    def size(self): ...
    @functools.cached_property
    def cached(self) -> int: ...
    def _private_method(self): ...
    def __len__(self): ...
    def args_first(*args): ...
    class Nested: ...
    type Inner = int
    @abc.abstractmethod
    @deprecated
    def gone(self): ...
type Alias = int
ints: TypeAlias = "list[int]"
@dataclasses.dataclass
class DC:
    """DC.

    Attributes:
        a: The a.
    """
    a: int
    _: KW_ONLY
    b: InitVar[str] = field(default="x")
    c: int = field(default_factory=int)
    d: list = field()
@dataclass
class WithInit:
    a: int
    def __init__(self): ...
`

func TestParseStatements(t *testing.T) {
	m := extractModule(t, parseSrc)
	if m.Doc == nil || m.Doc.Summary != "Module doc." {
		t.Errorf("module doc = %+v", m.Doc)
	}
	want := []string{"A", "Alias", "B", "DC", "FINAL", "LONG", "Plain", "Two", "WithInit", "counter", "decorated", "ints", "one", "self_x", "two"}
	if got := names(m.Symbols); !equal(got, want) {
		t.Errorf("symbols = %v\nwant      %v", got, want)
	}
	tests := []struct {
		path string
		kind apimodel.Kind
		code string
	}{
		{"LONG", apimodel.KindVariable, "LONG = ..."},
		{"FINAL", apimodel.KindVariable, "FINAL: Final[int] = 3"},
		{"counter", apimodel.KindVariable, "counter: int"},
		{"self_x", apimodel.KindVariable, "self_x = obj.attr = 3"},
		{"Alias", apimodel.KindType, "type Alias = int"},
		{"ints", apimodel.KindType, `ints: TypeAlias = "list[int]"`},
		{"Two", apimodel.KindClass, "class Two(Generic[T], metaclass=ABCMeta)"},
		{"Two.PLAIN", apimodel.KindProperty, "PLAIN = 1"},
		{"Two.count", apimodel.KindProperty, "count: ClassVar[int] = 0"},
		{"Two.size", apimodel.KindProperty, "size"},
		{"Two.cached", apimodel.KindProperty, "cached: int"},
		{"DC", apimodel.KindClass, "class DC"},
	}
	for _, tt := range tests {
		s := symbolNamed(t, m, tt.path)
		if s.Kind != tt.kind || s.Code != tt.code {
			t.Errorf("%s = %s %q, want %s %q", tt.path, s.Kind, s.Code, tt.kind, tt.code)
		}
	}
	checkTwo(t, symbolNamed(t, m, "Two"))
	checkDataclass(t, symbolNamed(t, m, "DC"), symbolNamed(t, m, "WithInit"))
	if s := symbolNamed(t, m, "one"); s.Doc == nil || s.Doc.Summary != "Doc on one line." {
		t.Errorf("one doc = %+v", s.Doc)
	}
	if s := symbolNamed(t, m, "counter"); s.Doc == nil || s.Doc.Summary != "A counter." {
		t.Errorf("counter doc = %+v", s.Doc)
	}
	if s := symbolNamed(t, m, "FINAL"); !s.Flags.Readonly || s.Type.String() != "int" {
		t.Errorf("FINAL = %+v %s", s.Flags, s.Type)
	}
	sig := symbolNamed(t, m, "two").Signatures[0]
	if sig.Code != `async def two(a, /, b: int = 1, *, c, **kw) -> "Two"` || len(sig.Params) != 4 || sig.Returns.Ref == "" {
		t.Errorf("two = %q %d params, returns %+v", sig.Code, len(sig.Params), sig.Returns)
	}
}
