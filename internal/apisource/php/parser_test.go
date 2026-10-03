package php

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestDeclarations(t *testing.T) {
	src := php(`
namespace A {
    use B\{C, D as E, function f, const G};
    use H\I, J as K;
    use function L\m;
    class X extends E implements C, K, I {}
    $x = new class { public function hidden() {} };
    $y = X::class;
    const TYPED = 1;
}
namespace {
    function &byRef(): void {}
    function () {};
    abstract readonly class R {
        public string $hooked { get => 'x'; }
        public $after;
        public final const string TC = 'a', TD = 'b';
        protected const PROT = 1;
        public function static(): static {}
        public function nop();
        case notEnum;
        { }
    }
}
`)
	pkg, diags := extractTree(t, map[string]string{"a.php": src}, apisource.Config{})
	if len(diags) > 0 {
		t.Errorf("diagnostics: %v", diags)
	}
	x := find(t, pkg, "p/A#X")
	if got := render(x.Extends[0]) + " " + render(x.Implements[0]) + " " + render(x.Implements[1]) + " " + render(x.Implements[2]); got != "E C K I" {
		t.Errorf("heritage = %q", got)
	}
	if x.Code != "class X extends E implements C, K, I" || len(x.Members) != 0 {
		t.Errorf("X = %q %v", x.Code, ids(x.Members))
	}
	if got := strings.Join(ids(pkg.Modules[0].Symbols), " "); got != "p/A#TYPED p/A#X" {
		t.Errorf("A symbols = %s", got)
	}
	if s := find(t, pkg, "p/global#byRef"); s.Signatures[0].Code != "function &byRef(): void" {
		t.Errorf("byRef = %q", s.Signatures[0].Code)
	}
	r := find(t, pkg, "p/global#R")
	if !r.Flags.Abstract || r.Code != "abstract readonly class R" {
		t.Errorf("R = %+v %q", r.Flags, r.Code)
	}
	want := "p/global#R.TC p/global#R.TD p/global#R.hooked p/global#R.after p/global#R.static p/global#R.nop"
	if got := strings.Join(ids(r.Members), " "); got != want {
		t.Errorf("R members = %s\nwant %s", got, want)
	}
	tc := find(t, pkg, "p/global#R.TC")
	if tc.Code != "public final const string TC = 'a'" || render(tc.Type) != "string" || !tc.Flags.Static || !tc.Flags.Readonly {
		t.Errorf("TC = %q %+v", tc.Code, tc.Flags)
	}
	if td := find(t, pkg, "p/global#R.TD"); td.Code != "public final const string TD = 'b'" {
		t.Errorf("TD = %q", td.Code)
	}
	if h := find(t, pkg, "p/global#R.hooked"); h.Code != "public string $hooked" || !h.Flags.Readonly {
		t.Errorf("hooked = %q %+v", h.Code, h.Flags)
	}
}

func TestMemberOrderAndCollisions(t *testing.T) {
	src := php(`
enum E: int {
    public function m() {}
    public $p;
    public function __construct() {}
    case B = 2;
    const C = 1;
    case A = 1;
}
class N {
    public function count() {}
    public $count;
    public $Count;
    /** @hidden */
    public $gone;
    /** @readonly */
    public $ro;
}
class Dup {}
class Dup {}
`)
	pkg, diags := extractTree(t, map[string]string{"a.php": src}, apisource.Config{})
	e := find(t, pkg, "p/global#E")
	want := "p/global#E.B p/global#E.A p/global#E.C p/global#E.p p/global#E.__construct p/global#E.m"
	if got := strings.Join(ids(e.Members), " "); got != want {
		t.Errorf("E members = %s\nwant %s", got, want)
	}
	n := find(t, pkg, "p/global#N")
	if got := strings.Join(ids(n.Members), " "); got != "p/global#N.$count p/global#N.Count p/global#N.ro p/global#N.count" {
		t.Errorf("N members = %s", got)
	}
	if !find(t, pkg, "p/global#N.ro").Flags.Readonly {
		t.Error("@readonly not applied")
	}
	if len(diags) != 1 || diags[0].Severity != apisource.Warning || !strings.Contains(diags[0].Message, "Dup is declared more than once") {
		t.Errorf("diagnostics = %v", diags)
	}
}

func TestPromotedAndHidden(t *testing.T) {
	src := php(`
namespace V;
/** @private */
function hiddenFn() {}
/** A point. */
final class Point {
    /**
     * @param int $x Across.
     * @param $y Down.
     */
    private function __construct(public int $x, public readonly $y = 0, private int $z = 0, protected(set) string $w = '') {}
    #[\Deprecated]
    public function old() {}
    /** @ignore */
    public function gone() {}
}
`)
	pkg, _ := extractTree(t, map[string]string{"src/v.php": src}, apisource.Config{})
	p := find(t, pkg, "p/V#Point")
	if got := strings.Join(ids(p.Members), " "); got != "p/V#Point.x p/V#Point.y p/V#Point.w p/V#Point.old" {
		t.Errorf("members = %s", got)
	}
	x := find(t, pkg, "p/V#Point.x")
	if x.Code != "public int $x" || x.Doc == nil || x.Doc.Summary != "Across." || x.Source.Line != 12 {
		t.Errorf("x = %q %+v %+v", x.Code, x.Doc, x.Source)
	}
	if y := find(t, pkg, "p/V#Point.y"); !y.Flags.Readonly || y.Type != nil || y.Doc.Summary != "Down." {
		t.Errorf("y = %+v %v", y.Flags, y.Type)
	}
	if d := find(t, pkg, "p/V#Point.old").Doc; d == nil || d.Deprecated == nil || *d.Deprecated != "" {
		t.Errorf("old doc = %+v", d)
	}
	if len(pkg.Modules[0].Symbols) != 1 {
		t.Errorf("symbols = %v", ids(pkg.Modules[0].Symbols))
	}
}

func TestParamShapes(t *testing.T) {
	src := php(`
function f(
    #[\SensitiveParameter] string $secret,
    ?int &$ref = null,
    A&B $both,
    A|(B&C) $dnf,
    $untyped = [1, 2],
    ...$rest,
): ?string {}
function noParens {}
`)
	pkg, _ := extractTree(t, map[string]string{"f.php": src}, apisource.Config{})
	sig := find(t, pkg, "p/global#f").Signatures[0]
	want := "function f(string $secret, ?int &$ref = null, A&B $both, A|(B&C) $dnf, $untyped = [1, 2], ...$rest): ?string"
	if sig.Code != want {
		t.Errorf("code = %q\nwant %q", sig.Code, want)
	}
	var got []string
	for _, p := range sig.Params {
		got = append(got, p.Name+":"+render(p.Type)+":"+p.Default)
	}
	if strings.Join(got, " ") != "secret:string: ref:int | null:null both:A & B: dnf:A | (B & C): untyped:unknown:[1, 2] rest:unknown:" {
		t.Errorf("params = %v", got)
	}
	if !sig.Params[5].Rest || !sig.Params[1].Optional {
		t.Error("rest/optional flags")
	}
	if s := find(t, pkg, "p/global#noParens"); len(s.Signatures[0].Params) != 0 {
		t.Error("noParens has params")
	}
}
