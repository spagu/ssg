package components

// Typed content components (GO-093).

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts a file in place, creating its directories.
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// library builds a component directory with a schema-carrying component, one
// with no schema at all, and one asset.
func library(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "youtube", "component.yaml"), `
description: Embed a video.
props:
  id: {type: string, required: true, description: The video id}
  title: {type: string, default: YouTube video}
  columns: {type: number, default: 1}
  autoplay: {type: bool, default: false}
  tags: {type: array}
  ratio: {type: string, enum: [16x9, 4x3], default: 16x9}
`)
	write(t, filepath.Join(dir, "youtube", "template.html"),
		`<figure class="yt yt--{{ .Props.ratio }}" data-cols="{{ .Props.columns }}">`+
			`<iframe title="{{ .Props.title }}" src="/e/{{ .Props.id }}"></iframe></figure>`)
	write(t, filepath.Join(dir, "youtube", "assets", "youtube.css"), ".yt{margin:0}")
	write(t, filepath.Join(dir, "youtube", "assets", "youtube.js"), "console.log('yt')")

	// No component.yaml: everything is allowed and nothing is checked.
	write(t, filepath.Join(dir, "bare", "template.html"), `<b>{{ .Props.text }}</b>`)
	return dir
}

func mustLoad(t *testing.T, dir string) *Set {
	t.Helper()
	set, warnings, err := Load(dir, template.FuncMap{"upper": strings.ToUpper})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	return set
}

// TestLoadReadsTheContract: the schema, the template and the assets.
func TestLoadReadsTheContract(t *testing.T) {
	set := mustLoad(t, library(t))
	if got := set.Names(); len(got) != 2 || got[0] != "bare" || got[1] != "youtube" {
		t.Fatalf("names = %v", got)
	}
	c, ok := set.Get("youtube")
	if !ok {
		t.Fatal("youtube missing")
	}
	if c.Description != "Embed a video." {
		t.Errorf("description = %q", c.Description)
	}
	if !c.Props["id"].Required || c.Props["title"].Default != "YouTube video" {
		t.Errorf("props = %+v", c.Props)
	}
	if len(c.StyleAssets()) != 1 || len(c.ScriptAssets()) != 1 {
		t.Errorf("assets = %v", c.Assets)
	}
	if _, ok := set.Get("nope"); ok {
		t.Error("a component nobody defined")
	}
}

// TestAbsentDirectoryIsNotAnError: components are opt-in, and a site with none
// must build exactly as it always did.
func TestAbsentDirectoryIsNotAnError(t *testing.T) {
	set, warnings, err := Load(filepath.Join(t.TempDir(), "nothing-here"), nil)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("Load: %v, %v", err, warnings)
	}
	if set.Len() != 0 || set.Names() != nil || set.Dir() != "" {
		t.Errorf("an absent directory should load as nothing: %+v", set)
	}
	out, res := set.Render("{{< anything >}}", nil, PolicyKeep, "", nil)
	if out != "{{< anything >}}" || len(res.Used) != 0 {
		t.Errorf("a nil set must leave content alone: %q", out)
	}
}

// TestLoadSkipsWhatIsNotAComponent, with a warning rather than a failed build:
// a stray folder is a mistake to point out, not a reason to stop.
func TestLoadSkipsWhatIsNotAComponent(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "good", "template.html"), "<p>ok</p>")
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Bad Name"), 0o750); err != nil {
		t.Fatal(err)
	}
	set, warnings, err := Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != 1 {
		t.Errorf("loaded %v", set.Names())
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v", warnings)
	}
}

// TestLoadRefusesASchemaThatCannotMeanAnything, at load time, where the author
// is looking.
func TestLoadRefusesASchemaThatCannotMeanAnything(t *testing.T) {
	cases := map[string]string{
		"unknown type":         "props:\n  a: {type: colour}\n",
		"enum on a number":     "props:\n  a: {type: number, enum: [1, 2]}\n",
		"required + default":   "props:\n  a: {required: true, default: x}\n",
		"default outside enum": "props:\n  a: {type: string, enum: [x, y], default: z}\n",
		"unparsable":           ":\n  : :\n",
	}
	for name, schema := range cases {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "c", "component.yaml"), schema)
		write(t, filepath.Join(dir, "c", "template.html"), "<p>x</p>")
		if _, _, err := Load(dir, nil); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
	// A template that does not parse is an error too.
	dir := t.TempDir()
	write(t, filepath.Join(dir, "c", "template.html"), "{{ .Props.a")
	if _, _, err := Load(dir, nil); err == nil {
		t.Error("an unparsable template must be refused")
	}
}

// TestFindCalls reads the syntax, and only the syntax.
func TestFindCalls(t *testing.T) {
	content := `Before {{< youtube id="abc" columns=3 autoplay=true >}} between
{{< bare text='single quoted' >}} and {{< plain >}} after.
Not a call: {{legacy}} or [bracket] or {{< >}} or {{<notaname!>}}.`
	calls := FindCalls(content)
	if len(calls) != 3 {
		t.Fatalf("found %d calls: %+v", len(calls), calls)
	}
	if calls[0].Name != "youtube" || calls[0].Args["id"] != "abc" ||
		calls[0].Args["columns"] != "3" || calls[0].Args["autoplay"] != "true" {
		t.Errorf("first call = %+v", calls[0])
	}
	if calls[1].Args["text"] != "single quoted" {
		t.Errorf("single quotes = %+v", calls[1])
	}
	if calls[2].Name != "plain" || len(calls[2].Args) != 0 {
		t.Errorf("bare call = %+v", calls[2])
	}
	// Raw is exactly what was matched, so an unresolved call goes back
	// untouched.
	if content[calls[0].Start:calls[0].End] != calls[0].Raw {
		t.Error("Raw does not match its own bounds")
	}
	// An empty value is a value.
	if got := FindCalls(`{{< c a="" >}}`); len(got) != 1 || got[0].Args["a"] != "" {
		t.Errorf("empty argument = %+v", got)
	}
}

// TestResolveTypesAndDefaults: what the template receives.
func TestResolveTypesAndDefaults(t *testing.T) {
	set := mustLoad(t, library(t))
	c, _ := set.Get("youtube")

	props, err := c.Resolve(FindCalls(`{{< youtube id="abc" columns=3 autoplay=true tags="a, b" >}}`)[0])
	if err != nil {
		t.Fatal(err)
	}
	if props["id"] != "abc" || props["columns"] != 3 || props["autoplay"] != true {
		t.Errorf("props = %#v", props)
	}
	if tags, ok := props["tags"].([]string); !ok || len(tags) != 2 || tags[1] != "b" {
		t.Errorf("tags = %#v", props["tags"])
	}
	// Defaults fill what the call left out.
	if props["title"] != "YouTube video" || props["ratio"] != "16x9" {
		t.Errorf("defaults = %#v", props)
	}
	// A float where a number is asked for.
	props, err = c.Resolve(FindCalls(`{{< youtube id="x" columns=1.5 >}}`)[0])
	if err != nil || props["columns"] != 1.5 {
		t.Errorf("float = %#v, %v", props["columns"], err)
	}
}

// TestResolveRefusesWhatTheSchemaForbids, and says which prop and why.
func TestResolveRefusesWhatTheSchemaForbids(t *testing.T) {
	set := mustLoad(t, library(t))
	c, _ := set.Get("youtube")
	cases := map[string]string{
		`{{< youtube >}}`:                       `"id" is required`,
		`{{< youtube id="x" columns=many >}}`:   "is not a number",
		`{{< youtube id="x" autoplay=maybe >}}`: "is not true or false",
		`{{< youtube id="x" ratio="square" >}}`: "is not one of: 16x9, 4x3",
		`{{< youtube id="x" nope="y" >}}`:       "is not one of its props",
	}
	for call, wants := range cases {
		_, err := c.Resolve(FindCalls(call)[0])
		if err == nil || !strings.Contains(err.Error(), wants) {
			t.Errorf("%s: %v (want %q)", call, err, wants)
		}
	}
}

// TestAComponentWithNoSchemaTakesAnything, because it has promised nothing.
func TestAComponentWithNoSchemaTakesAnything(t *testing.T) {
	set := mustLoad(t, library(t))
	c, _ := set.Get("bare")
	props, err := c.Resolve(FindCalls(`{{< bare text="hello" extra="fine" >}}`)[0])
	if err != nil {
		t.Fatal(err)
	}
	if props["text"] != "hello" || props["extra"] != "fine" {
		t.Errorf("props = %#v", props)
	}
}

// TestRenderReplacesCallsAndReportsUse.
func TestRenderReplacesCallsAndReportsUse(t *testing.T) {
	set := mustLoad(t, library(t))
	content := "Watch:\n\n{{< youtube id=\"abc\" ratio=\"4x3\" >}}\n\nand {{< bare text=\"hi\" >}}.\n"
	out, res := set.Render(content, nil, PolicyKeep, "page.md", nil)
	if !strings.Contains(out, `class="yt yt--4x3"`) || !strings.Contains(out, "<b>hi</b>") {
		t.Errorf("render = %q", out)
	}
	if strings.Contains(out, "{{<") {
		t.Errorf("a call survived: %q", out)
	}
	if len(res.Used) != 2 || res.Used[0] != "youtube" {
		t.Errorf("used = %v", res.Used)
	}
	if len(res.Warnings) != 0 || res.Err != nil {
		t.Errorf("warnings = %v, err = %v", res.Warnings, res.Err)
	}
	// The text around the calls is untouched.
	if !strings.HasPrefix(out, "Watch:\n\n") || !strings.HasSuffix(out, ".\n") {
		t.Errorf("surrounding text changed: %q", out)
	}
}

// TestRenderEscapesPropsFromContent: a prop is content, and html/template is
// what stands between it and the page.
func TestRenderEscapesPropsFromContent(t *testing.T) {
	set := mustLoad(t, library(t))
	out, _ := set.Render(`{{< youtube id="x" title="</iframe><script>alert(1)</script>" >}}`, nil, PolicyKeep, "", nil)
	if strings.Contains(out, "<script>alert(1)") {
		t.Errorf("a prop broke out of its attribute:\n%s", out)
	}
}

// TestUnresolvedCallsFollowThePolicy: keep leaves them visible, drop takes them
// out, strict stops the build. None of them invents markup.
func TestUnresolvedCallsFollowThePolicy(t *testing.T) {
	set := mustLoad(t, library(t))
	content := "a {{< nosuch >}} b {{< youtube >}} c"

	kept, res := set.Render(content, nil, PolicyKeep, "page.md", nil)
	if kept != content {
		t.Errorf("keep should leave the calls where they are: %q", kept)
	}
	if len(res.Warnings) != 2 || res.Err != nil {
		t.Errorf("keep: warnings=%v err=%v", res.Warnings, res.Err)
	}
	// Every message names the call, which is what an author can search for.
	for _, w := range res.Warnings {
		if !strings.Contains(w, "{{<") || !strings.Contains(w, "page.md") {
			t.Errorf("message is not actionable: %q", w)
		}
	}

	// drop removes the call that was made wrongly, and leaves the one that
	// names nothing: that is text, not a broken call.
	dropped, _ := set.Render(content, nil, PolicyDrop, "", nil)
	if dropped != "a {{< nosuch >}} b  c" {
		t.Errorf("drop = %q", dropped)
	}

	_, strict := set.Render(content, nil, PolicyStrict, "", nil)
	if strict.Err == nil || !strings.Contains(strict.Err.Error(), "youtube") {
		t.Errorf("strict must fail on a call made wrongly: %v", strict.Err)
	}

	// A call naming a component the site does not have never fails a build,
	// under any policy: documentation quoting a call is the ordinary case.
	for _, policy := range []string{PolicyDrop, PolicyKeep, PolicyStrict} {
		out, res := set.Render("prose about {{< nosuch >}} only", nil, policy, "", nil)
		if out != "prose about {{< nosuch >}} only" {
			t.Errorf("%s changed text that is not a call: %q", policy, out)
		}
		if res.Err != nil {
			t.Errorf("%s failed on text that is not a call: %v", policy, res.Err)
		}
		if len(res.Warnings) != 1 {
			t.Errorf("%s: it is still worth one warning: %v", policy, res.Warnings)
		}
	}
}

// TestRenderWrapsOutput: the generator uses this to protect component markup
// from the sanitizer, and to mark which component produced what.
func TestRenderWrapsOutput(t *testing.T) {
	set := mustLoad(t, library(t))
	out, _ := set.RenderMarked(`{{< bare text="x" >}}`, nil, PolicyKeep, "",
		func(name, html string) string { return "[" + name + ":" + html + "]" })
	if out != "[bare:<b>x</b>]" {
		t.Errorf("out = %q", out)
	}
	// Render's simpler wrapper is the same thing without the name.
	out, _ = set.Render(`{{< bare text="x" >}}`, nil, PolicyKeep, "",
		func(html string) string { return "(" + html + ")" })
	if out != "(<b>x</b>)" {
		t.Errorf("out = %q", out)
	}
}

// TestRenderLeavesContentWithoutCallsAlone, without doing any work.
func TestRenderLeavesContentWithoutCallsAlone(t *testing.T) {
	set := mustLoad(t, library(t))
	content := "Plain prose with {{legacy}} and [bracket] and nothing else."
	out, res := set.Render(content, nil, PolicyKeep, "", nil)
	if out != content || len(res.Used) != 0 {
		t.Errorf("out = %q", out)
	}
}

// TestRenderReportsATemplateThatFails at run time, naming the component.
func TestRenderReportsATemplateThatFails(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "boom", "template.html"), `{{ len .Props.missing }}`)
	set := mustLoad(t, dir)
	_, res := set.Render(`{{< boom >}}`, nil, PolicyKeep, "", nil)
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "failed to render") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

// TestManifestIsTheContract a person or an agent reads before writing a call.
func TestManifestIsTheContract(t *testing.T) {
	set := mustLoad(t, library(t))
	m := set.Manifest()
	if m.Schema != ManifestSchema || len(m.Components) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
	var yt ManifestComponent
	for _, c := range m.Components {
		if c.Name == "youtube" {
			yt = c
		}
	}
	if yt.Description == "" || len(yt.Props) != 6 {
		t.Errorf("youtube = %+v", yt)
	}
	if yt.Example != `{{< youtube id="…" >}}` {
		t.Errorf("example = %q — it should show only what is required", yt.Example)
	}
	for _, p := range yt.Props {
		if p.Type == "" {
			t.Errorf("prop %q has no type in the manifest", p.Name)
		}
		if p.Name == "id" && (!p.Required || p.Description == "") {
			t.Errorf("id = %+v", p)
		}
	}
	data, err := set.EncodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	// The angle brackets must survive as themselves; JSON still escapes the
	// quotes, which is JSON's business.
	if !strings.Contains(string(data), `"{{< youtube id=`) || strings.Contains(string(data), `\u003c`) {
		t.Errorf("the example must be copyable, not escaped:\n%s", data)
	}
}

// TestManifestExamplesCoverEveryPropShape.
func TestManifestExamplesCoverEveryPropShape(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "c", "component.yaml"), `
props:
  n: {type: number, required: true}
  b: {type: bool, required: true}
  a: {type: array, required: true}
  e: {type: string, required: true, enum: [one, two]}
`)
	write(t, filepath.Join(dir, "c", "template.html"), "<p>x</p>")
	set := mustLoad(t, dir)
	got := set.Manifest().Components[0].Example
	for _, want := range []string{`n=1`, `b=true`, `a="a, b"`, `e="one"`} {
		if !strings.Contains(got, want) {
			t.Errorf("example %q lacks %q", got, want)
		}
	}
}

// TestNilSetIsHarmless: a site with no components calls the same methods, and
// every one of them answers as "none".
func TestNilSetIsHarmless(t *testing.T) {
	var set *Set
	if set.Len() != 0 || set.Names() != nil || set.Dir() != "" {
		t.Error("a nil set should report nothing")
	}
	if _, ok := set.Get("anything"); ok {
		t.Error("a nil set has no components")
	}
	out, res := set.RenderMarked("{{< x >}}", nil, PolicyStrict, "", nil)
	if out != "{{< x >}}" || res.Err != nil {
		t.Errorf("a nil set must leave content alone: %q %v", out, res.Err)
	}
	if m := set.Manifest(); len(m.Components) != 0 || m.Schema != ManifestSchema {
		t.Errorf("manifest = %+v", m)
	}
	if _, err := set.EncodeManifest(); err != nil {
		t.Errorf("encoding an empty manifest: %v", err)
	}
}

// TestLoadReportsAnUnreadableDirectory rather than treating it as absent.
func TestLoadReportsAnUnreadableDirectory(t *testing.T) {
	// A file where the components directory should be is not "no components".
	dir := t.TempDir()
	path := filepath.Join(dir, "components")
	write(t, path, "not a directory")
	if _, _, err := Load(path, nil); err == nil {
		t.Error("a file in the directory's place must be reported")
	}
}

// TestValidName keeps a component addressable and safe to put in a path.
func TestValidName(t *testing.T) {
	for _, ok := range []string{"a", "youtube", "call-out", "a_b", "h2"} {
		if !validName(ok) {
			t.Errorf("%q should be a valid name", ok)
		}
	}
	for _, bad := range []string{"", "A", "a b", "a/b", "..", "a.b", "ünicode"} {
		if validName(bad) {
			t.Errorf("%q should not be a valid name", bad)
		}
	}
}

// TestAssetsAreOptionalAndSorted.
func TestAssetsAreOptionalAndSorted(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "c", "template.html"), "<p>x</p>")
	write(t, filepath.Join(dir, "c", "assets", "z.css"), "z{}")
	write(t, filepath.Join(dir, "c", "assets", "a.css"), "a{}")
	write(t, filepath.Join(dir, "c", "assets", "nested", "b.js"), "//b")
	write(t, filepath.Join(dir, "c", "assets", "readme.txt"), "notes")
	set := mustLoad(t, dir)
	c, _ := set.Get("c")
	if len(c.Assets) != 4 || c.Assets[0] != "a.css" {
		t.Errorf("assets = %v (want them sorted)", c.Assets)
	}
	if got := c.StyleAssets(); len(got) != 2 {
		t.Errorf("styles = %v", got)
	}
	if got := c.ScriptAssets(); len(got) != 1 || got[0] != "nested/b.js" {
		t.Errorf("scripts = %v", got)
	}
	// A component with no assets directory has no assets and no error.
	write(t, filepath.Join(dir, "d", "template.html"), "<p>y</p>")
	set = mustLoad(t, dir)
	d, _ := set.Get("d")
	if len(d.Assets) != 0 {
		t.Errorf("assets = %v", d.Assets)
	}
}

// TestSetDirIsWhereItCameFrom.
func TestSetDirIsWhereItCameFrom(t *testing.T) {
	dir := library(t)
	if got := mustLoad(t, dir).Dir(); got != dir {
		t.Errorf("Dir = %q, want %q", got, dir)
	}
}

// TestCallsInCodeAreNotCalls: documentation showing how to write a call is the
// ordinary reason the syntax appears in a file, and expanding it there would
// make a component impossible to document on the site that has it.
func TestCallsInCodeAreNotCalls(t *testing.T) {
	set := mustLoad(t, library(t))
	content := "Write `{{< bare text=\"x\" >}}` to embed one.\n\n" +
		"```\n{{< bare text=\"in a fence\" >}}\n```\n\n" +
		"    {{< bare text=\"indented\" >}}\n\n" +
		"But {{< bare text=\"real\" >}} is a call.\n"
	calls := FindCalls(content)
	if len(calls) != 1 || calls[0].Args["text"] != "real" {
		t.Fatalf("found %d calls: %+v", len(calls), calls)
	}
	out, res := set.Render(content, nil, PolicyStrict, "", nil)
	if strings.Count(out, "{{<") != 3 {
		t.Errorf("the quoted calls were touched:\n%s", out)
	}
	if !strings.Contains(out, "<b>real</b>") {
		t.Errorf("the real call did not render:\n%s", out)
	}
	if res.Err != nil || len(res.Warnings) != 0 {
		t.Errorf("quoted calls must not warn: %v %v", res.Err, res.Warnings)
	}
}

// TestFencedBlockAtTheStartOfAFile is its own case: the fence has no newline
// before it to anchor on.
func TestFencedBlockAtTheStartOfAFile(t *testing.T) {
	content := "```\n{{< bare text=\"x\" >}}\n```\nthen {{< bare text=\"y\" >}}\n"
	calls := FindCalls(content)
	if len(calls) != 1 || calls[0].Args["text"] != "y" {
		t.Errorf("calls = %+v", calls)
	}
}
