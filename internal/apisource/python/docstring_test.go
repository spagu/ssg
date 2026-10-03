package python

import (
	"fmt"
	"strings"
	"testing"
)

// render prints a docInfo as "field=value" lines, empty fields left out.
func render(info *docInfo) string {
	var b strings.Builder
	add := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	d := info.doc
	add("summary", d.Summary)
	add("body", d.Body)
	add("returns", d.Returns)
	add("rtype", info.returnType)
	add("throws", strings.Join(d.Throws, "|"))
	add("examples", strings.Join(d.Examples, "|"))
	if d.Deprecated != nil {
		add("deprecated", "!"+*d.Deprecated)
	}
	add("since", d.Since)
	add("see", strings.Join(d.See, "|"))
	for _, tag := range d.Tags {
		add("tag", tag.Name+":"+tag.Text)
	}
	for _, p := range info.params {
		add("param", p.name+":"+p.typ+":"+p.text)
	}
	for _, p := range info.attrs {
		add("attr", p.name+":"+p.typ+":"+p.text)
	}
	return b.String()
}

func TestParseDocstring(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"one line", "Does a thing.", "summary=Does a thing.\n"},
		{"pep257", "Summary\n    spans.\n\n    Body line\n      indented.\n    ", "summary=Summary spans.\nbody=Body line\n  indented.\n"},
		{"tabs", "S.\n\n\tBody.", "summary=S.\nbody=Body.\n"},
		{"google", `Sum.

    Args:
        x (int, optional): The x.
        *args: More
            values.
        plain
    Returns:
        The result.
    Yields:
        int: Numbers.
    Raises:
        ValueError: Bad.
    Note:
        Careful.
    Todo:
        Later.
    Deprecated:
        Use y.
    Attributes:
        a (str): An a.
    See Also:
        other
    Example:
        run(1)
    `, "summary=Sum.\nbody=**Note:** Careful.\nreturns=The result.\n\nNumbers.\nrtype=int\nthrows=ValueError: Bad.\nexamples=```python\nrun(1)\n```\ndeprecated=!Use y.\nsee={@link other}\ntag=todo:Later.\nparam=x:int:The x.\nparam=args::More\nvalues.\nparam=plain::\nattr=a:str:An a.\n"},
		{"numpy", `Sum.

    Parameters
    ----------
    x, y : int, default 0
        Coords.
    flag
        A flag.

    Returns
    -------
    count : int
        How many.
    rest : list
        The rest.

    Raises
    ------
    KeyError
        Missing.

    See Also
    --------
    :func:` + "`other`" + ` : Other one.
    a, b
    not a name : Text.

    Notes
    -----
    Some notes.
    `, "summary=Sum.\nbody=**Notes:** Some notes.\nreturns=- `count`: How many.\n- `rest`: The rest.\nthrows=KeyError: Missing.\nsee={@link other}: Other one.|{@link a}|{@link b}|not a name: Text.\nparam=x:int:Coords.\nparam=y:int:Coords.\nparam=flag::A flag.\n"},
		{"numpy single return", "S.\n\nReturns\n-------\nstr\n    The text.\n", "summary=S.\nreturns=The text.\nrtype=str\n"},
		{"rest", `Sum.

    :param int x: The x.
    :param y: The y,
        continued.
    :type y: list[str]
    :ivar z: A z.
    :vartype z: int
    :returns: The result.
    :rtype: bool
    :raises ValueError: Bad.
    :meta private:
    `, "summary=Sum.\nreturns=The result.\nrtype=bool\nthrows=ValueError: Bad.\ntag=meta:private\nparam=x:int:The x.\nparam=y:list[str]:The y,\ncontinued.\nattr=z:int:A z.\n"},
		{"directives", `Sum.

    .. deprecated:: 2.0
       Use :class:` + "`~pkg.New`" + `.
    .. deprecated::
    .. versionadded:: 1.1
    .. versionchanged:: 1.5 Faster.
    .. note:: Mind it.
    .. warning::
       Hot.
    .. seealso:: :func:` + "`a`" + `
    .. unknown:: kept
    `, "summary=Sum.\nbody=.. unknown:: kept\n\n**Note:** Mind it.\n\n**Warning:** Hot.\ndeprecated=!\nsince=1.1\nsee={@link a}\ntag=versionchanged:1.5 Faster.\n"},
		{"doctest and literal", `Sum.

    >>> f(1)
    2

    Use it like this::

        f(2)

    Or::

        f(3)

    ::

        f(4)

    .. code-block:: rust

        fn main() {}

    ` + "```js\n    >>> not a doctest\n    ```", "summary=Sum.\nbody=Use it like this:\n\n```python\nf(2)\n```\nOr:\n\n```python\nf(3)\n```\n```python\nf(4)\n```\n```rust\nfn main() {}\n```\n```js\n>>> not a doctest\n```\nexamples=```python\n>>> f(1)\n2\n```\n"},
		{"examples prose", "S.\n\nExamples:\n    Call it::\n\n        f()\n\n    >>> g()\n", "summary=S.\nexamples=Call it:\n\n```python\nf()\n```\n```python\n>>> g()\n```\n"},
		{"markup", "See :class:`Foo`, :meth:`Foo.bar()`, :func:`title <pkg.f>`, :py:attr:`~a.b`, :exc:`!E`, :code:`x`, ``lit``, [`Foo`], [`L`](url).", "summary=See {@link Foo}, {@link Foo.bar}, {@link pkg.f | title}, {@link a.b | b}, `E`, `x`, `lit`, {@link Foo}, [`L`](url).\n"},
		{"literal only", "Sum.\n\n    ::\n\n        x\n", "summary=Sum.\nbody=```python\nx\n```\n"},
		{"fence first", "```py\nx\n```", "body=```py\nx\n```\n"},
		{"empty section", "S.\n\nReturns:\nNote:", "summary=S.\nbody=**Note:** \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(parseDocstring(tt.raw)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestCleanType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"int, optional", "int"},
		{"dict(str, int), default: {}", "dict(str, int)"},
		{"Optional", ""},
		{" list ", "list"},
	}
	for _, tt := range tests {
		if got := cleanType(tt.in); got != tt.want {
			t.Errorf("cleanType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
