package tstype

import "testing"

// TestParseRoundTrip checks that each construct parses and renders to its
// canonical text through apimodel.TypeRef.String.
func TestParseRoundTrip(t *testing.T) {
	cases := []struct{ in, want string }{
		// names and generics
		{"string", "string"},
		{"  Foo  ", "Foo"},
		{"ns.Type", "ns.Type"},
		{"Map<K, V>", "Map<K, V>"},
		{"Map<string, Array<number>>", "Map<string, Array<number>>"},
		{"Promise<Map<K, Set<V>>>", "Promise<Map<K, Set<V>>>"},
		{"Foo<A,>", "Foo<A>"},
		{"readonly", "readonly"},
		// unions, intersections, grouping
		{"A | B", "A | B"},
		{"| A | B", "A | B"},
		{"A & B | C", "A & B | C"},
		{"& A & B", "A & B"},
		{"(A | B)[]", "(A | B)[]"},
		{"(A)", "A"},
		// arrays and tuples
		{"T[]", "T[]"},
		{"T[][]", "T[][]"},
		{"readonly T[]", "T[]"},
		{"readonly [A, B]", "[A, B]"},
		{"[]", "[]"},
		{"[A, B]", "[A, B]"},
		{"[x: A, y: B]", "[A, B]"},
		{"[A, B?]", "[A, B?]"},
		{"[A, ...B[]]", "[A, ...B[]]"},
		{"[x: A, y?: B]", "[x: A, y?: B]"},
		// literals
		{"'a'", "'a'"},
		{`"a" | "b"`, `"a" | "b"`},
		{"`plain`", "`plain`"},
		{`'it\'s'`, `'it\'s'`},
		{"`a\\`${'}'}`", "`a\\`${'}'}`"},
		{"42", "42"},
		{"-1", "-1"},
		{"10n", "10n"},
		{"true | false", "true | false"},
		{"null | undefined", "null | undefined"},
		// functions
		{"() => void", "() => void"},
		{"(a: A, b?: B, ...rest: C[]) => R", "(a: A, b?: B, ...rest: C[]) => R"},
		{"<T>(x: T) => T", "(x: T) => T"},
		{"(a, b) => void", "(a: any, b: any) => void"},
		{"({ a, b }: Opts) => void", "({ a, b }: Opts) => void"},
		{"(this: Window) => void", "(this: Window) => void"},
		{"new (x: A) => B", "(x: A) => B"},
		{"abstract new () => B", "() => B"},
		{"A | () => void", "A | () => void"},
		{"(() => void)[]", "(() => void)[]"},
		// objects
		{"{}", "{  }"},
		{"{ a: A; b?: B, readonly c: C }", "{ a: A; b?: B; c: C }"},
		{"{ a: A\n b: B }", "{ a: A; b: B }"},
		{"{ m(x: A): B }", "{ m: (x: A) => B }"},
		{"{ m?<T>(x: T) }", "{ m?: (x: T) => any }"},
		{"{ [key: string]: V }", "{ [key: string]: V }"},
		{"{ [Symbol.iterator](): It }", "{ [Symbol.iterator]: () => It }"},
		{"{ (x: A): B; new (x: A): C }", "{ (): (x: A) => B; new: (x: A) => C }"},
		{"{ get x(): T; set y(v: U); set z() }", "{ x: T; y: U; z: any }"},
		{"{ 'quoted': A; 1: B; readonly: C }", "{ 'quoted': A; 1: B; readonly: C }"},
		{"{ get: G }", "{ get: G }"},
		// operators kept verbatim
		{"keyof T", "keyof T"},
		{"keyof  typeof   obj", "keyof typeof obj"},
		{"typeof x.y<T>", "typeof x.y<T>"},
		{"typeof x[number]", "typeof x[number]"},
		{"unique symbol", "unique symbol"},
		{"T[K]", "T[K]"},
		{"T[K][]", "T[K][]"},
		{"A extends B ? C : D", "A extends B ? C : D"},
		{"T extends (infer U)[] ? U : never", "T extends (infer U)[] ? U : never"},
		{"T extends [infer H extends string, ...infer R] ? H : never", "T extends [infer H extends string, ...infer R] ? H : never"},
		{"{ [K in keyof T]: V }", "{ [K in keyof T]: V }"},
		{"{ -readonly [K in T as `get${K}`]+?: V; }", "{ -readonly [K in T as `get${K}`]+?: V; }"},
		{"{ readonly [K in T] }", "{ readonly [K in T] }"},
		{"`a${B}c`", "`a${B}c`"},
		{"`a${ `x${ {} }` }c`", "`a${ `x${ {} }` }c`"},
		{"(x: unknown) => asserts x is  T", "(x: unknown) => asserts x is T"},
		{"(x: unknown) => asserts x", "(x: unknown) => asserts x"},
		{"(x: unknown) => x is T", "(x: unknown) => x is T"},
		{"this is T", "this is T"},
		{"<in out T, const U>() => T", "() => T"},
		// whitespace and trailing semicolons
		{"Map<\n  K,\n  V\n>;;", "Map<K, V>"},
	}
	for _, c := range cases {
		got, err := Parse(c.in, nil)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error %v", c.in, err)
			continue
		}
		if s := got.String(); s != c.want {
			t.Errorf("Parse(%q).String() = %q, want %q", c.in, s, c.want)
		}
	}
}

// TestParseJSDoc checks the JSDoc-only forms accepted in comment types.
func TestParseJSDoc(t *testing.T) {
	cases := []struct{ in, want string }{
		{"?string", "string | null"},
		{"?(A | B)", "A | B | null"},
		{"!Object", "Object"},
		{"string=", "string"},
		{"*", "any"},
		{"?", "unknown"},
		{"Array<?>", "Array<unknown>"},
		{"function(string, number): boolean", "(p0: string, p1: number) => boolean"},
		{"function(this:T, ...number)", "(this: T, ...p1: number) => unknown"},
		{"function(new:Foo, string=)", "(new: Foo, p1?: string) => unknown"},
		{"function()", "() => unknown"},
		{"Array.<string>", "Array<string>"},
		{"Object.<string, number>", "Object<string, number>"},
		{"{a: number, b}", "{ a: number; b: unknown }"},
	}
	for _, c := range cases {
		got, err := Parse(c.in, nil)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error %v", c.in, err)
			continue
		}
		if s := got.String(); s != c.want {
			t.Errorf("Parse(%q).String() = %q, want %q", c.in, s, c.want)
		}
	}
}

// TestStripOptional checks the trailing `=` marker.
func TestStripOptional(t *testing.T) {
	cases := []struct {
		in, want string
		opt      bool
	}{
		{"string=", "string", true},
		{" Foo<T> = ", "Foo<T>", true},
		{"string", "string", false},
		{"() => void", "() => void", false},
		{"a==", "a==", false},
	}
	for _, c := range cases {
		got, opt := StripOptional(c.in)
		if got != c.want || opt != c.opt {
			t.Errorf("StripOptional(%q) = %q, %v; want %q, %v", c.in, got, opt, c.want, c.opt)
		}
	}
}
