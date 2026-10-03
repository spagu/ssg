package apidoc

import (
	"reflect"
	"testing"
)

// TestFindLinks covers every link form and the places links are ignored.
func TestFindLinks(t *testing.T) {
	tests := []struct {
		name, md string
		want     []Link
	}{
		{"none", "plain text", nil},
		{"simple", "See {@link Foo}.", []Link{{Target: "Foo", Start: 4, End: 15}}},
		{"pipe", "{@link Foo.bar | the bar}", []Link{{Target: "Foo.bar", Text: "the bar", End: 25}}},
		{"space text", "{@link Foo the foo}", []Link{{Target: "Foo", Text: "the foo", End: 19}}},
		{"linkcode", "{@linkcode Foo}", []Link{{Target: "Foo", Code: true, End: 15}}},
		{"linkplain", "{@linkplain Foo|x}", []Link{{Target: "Foo", Text: "x", End: 18}}},
		{"multiline", "{@link\nFoo\ntext}", []Link{{Target: "Foo", Text: "text", End: 16}}},
		{"two", "{@link A} and {@link B}", []Link{{Target: "A", End: 9}, {Target: "B", Start: 14, End: 23}}},
		{"empty target", "{@link } {@link | x}", nil},
		{"unknown tag", "{@code x} {@linkfoo x} {@linkX y}", nil},
		{"unterminated", "{@link Foo", nil},
		{"code span", "`{@link A}` {@link B}", []Link{{Target: "B", Start: 12, End: 21}}},
		{"double code span", "`` a ` {@link A} `` x", nil},
		{"unmatched backtick", "` {@link A}", []Link{{Target: "A", Start: 2, End: 11}}},
		{"mismatched runs", "`` {@link A} `", []Link{{Target: "A", Start: 3, End: 12}}},
		{"fence", "```js\n{@link A}\n```\n{@link B}", []Link{{Target: "B", Start: 20, End: 29}}},
		{"tilde fence unclosed", "~~~\n{@link A}", nil},
		{"inner marker keeps fence", "```\n~~~\n``` js\n{@link A}\n````\n{@link B}", []Link{{Target: "B", Start: 30, End: 39}}},
		{"after fence close", "a\n```\nx\n```\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindLinks(tt.md)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestRewriteLinks covers resolved, unresolved and duplicate targets.
func TestRewriteLinks(t *testing.T) {
	resolve := func(target string) (string, bool) {
		if target == "Foo" {
			return "foo.html#Foo", true
		}
		return "", false
	}
	tests := []struct {
		name, md, out string
		unresolved    []string
	}{
		{"no links", "plain `{@link Foo}`", "plain `{@link Foo}`", nil},
		{"resolved", "See {@link Foo}.", "See [Foo](foo.html#Foo).", nil},
		{"resolved text", "{@link Foo | the foo}", "[the foo](foo.html#Foo)", nil},
		{"resolved code", "{@linkcode Foo}", "[`Foo`](foo.html#Foo)", nil},
		{"unresolved", "{@link Bar} {@link Bar x} {@linkcode Baz}", "Bar x `Baz`", []string{"Bar", "Baz"}},
		{"mixed", "a {@link Foo} b {@link Qux}\n```\n{@link Qux}\n```", "a [Foo](foo.html#Foo) b Qux\n```\n{@link Qux}\n```", []string{"Qux"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, unresolved := RewriteLinks(tt.md, resolve)
			if out != tt.out || !reflect.DeepEqual(unresolved, tt.unresolved) {
				t.Errorf("got %q %q\nwant %q %q", out, unresolved, tt.out, tt.unresolved)
			}
		})
	}
	if out, un := RewriteLinks("{@link Foo}", nil); out != "Foo" || !reflect.DeepEqual(un, []string{"Foo"}) {
		t.Errorf("nil resolve: %q %q", out, un)
	}
}
