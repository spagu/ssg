package php

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestMemberModifiers(t *testing.T) {
	src := php(`
use function X\{a, b};
use const X\C;
class M {
    use T1, T2 { T1::f as g; }
    public private(set) string $set = '';
    protected static $prot;
    final public static function s() {}
    abstract public function abs(): void;
    public const NOVALUE;
    public static ?M $one = null, $two;
}
class Dup {}
class Dup {}
class Dup {}
`)
	pkg, diags := extractTree(t, map[string]string{"m.php": src}, apisource.Config{})
	m := find(t, pkg, "p/global#M")
	if got := strings.Join(ids(m.Members), " "); got != "p/global#M.set p/global#M.one p/global#M.two p/global#M.s p/global#M.abs" {
		t.Errorf("members = %s", got)
	}
	if s := find(t, pkg, "p/global#M.set"); s.Code != "public private(set) string $set = ''" {
		t.Errorf("set = %q", s.Code)
	}
	if s := find(t, pkg, "p/global#M.two"); s.Code != "public static ?M $two" || !s.Flags.Static || render(s.Type) != "M→p/global#M | null" {
		t.Errorf("two = %q %+v", s.Code, s.Flags)
	}
	if !find(t, pkg, "p/global#M.abs").Flags.Abstract || !find(t, pkg, "p/global#M.s").Flags.Static {
		t.Error("abstract/static flags")
	}
	if len(diags) != 2 || diags[0].Line >= diags[1].Line {
		t.Errorf("diagnostics = %v", diags)
	}
}

func TestTruncatedDeclarations(t *testing.T) {
	for _, src := range []string{
		"<?php class",
		"<?php class X extends",
		"<?php class X { public",
		"<?php class X { public int",
		"<?php class X { public $a = [1",
		"<?php class X { const A = ",
		"<?php class X { function f(",
		"<?php enum E { case A",
		"<?php namespace A",
		"<?php use A\\{B",
		"<?php use function",
		"<?php function f(): int",
		"<?php {",
	} {
		toks, err := tokenize(src)
		if err != nil {
			t.Fatal(err)
		}
		p := &parser{src: src, file: "x.php", toks: toks, scope: newScope("")}
		p.statements(false) // must not panic or loop
	}
}
