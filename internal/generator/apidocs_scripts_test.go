package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectAPITools(t *testing.T) {
	plain := "<html><body><p>x</p></body></html>"
	if injectAPITools(plain) != plain {
		t.Error("a page without a marker must not change")
	}
	page := `<html><body><div class="ssg-playground" data-ssg-playground data-package="p"></div></body></html>`
	got := injectAPITools(page)
	if !strings.Contains(got, "data-ssg-playground-script") || !strings.Contains(got, apiToolsStyleAttr) ||
		strings.Contains(got, "data-ssg-tryit-script") {
		t.Errorf("playground page: %s", got[len(got)-200:])
	}
	if !strings.HasSuffix(got, "\n</body></html>") {
		t.Error("the tools go before </body>")
	}
	if injectAPITools(got) != got {
		t.Error("injecting twice must not add a second copy")
	}
	both := `<div data-ssg-playground></div><div class="ssg-tryit" data-ssg-tryit="{}"></div>`
	got = injectAPITools(both)
	if strings.Count(got, "<style "+apiToolsStyleAttr) != 1 || !strings.Contains(got, "data-ssg-tryit-script") ||
		!strings.HasPrefix(got, both) {
		t.Errorf("no body: %s", got)
	}
	// A name that only starts like a marker is not one.
	if odd := `<p data-ssg-tryit-later>x</p>`; injectAPITools(odd) != odd {
		t.Error("a longer attribute is not a marker")
	}
}

// The scripts are inlined into <script> and the styles into <style>: a
// closing tag inside either would end the element early.
func TestAPIToolsAreInlineSafe(t *testing.T) {
	for name, src := range map[string]string{"playground": playgroundScript, "tryit": tryItScript} {
		if strings.Contains(strings.ToLower(src), "</script") {
			t.Errorf("%s contains a closing script tag", name)
		}
	}
	if strings.Contains(strings.ToLower(apiToolsStyle), "</style") {
		t.Error("the styles contain a closing style tag")
	}
}

// The scripts' request and runner builders, run in Node where it exists.
func TestAPIToolsRuntime(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	for name, src := range map[string]string{"playground.cjs": playgroundScript, "tryit.cjs": tryItScript} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := `
const assert = require('assert');
const t = require('./tryit.cjs'), p = require('./playground.cjs');
const op = {method: 'POST', path: '/pets/{id}', body: {type: 'application/json', required: true},
  params: [{name: 'id', in: 'path', required: true}, {name: 'q', in: 'query'}, {name: 'X-Trace', in: 'header'},
           {name: 'session', in: 'cookie'}],
  security: [{name: 'key', type: 'apiKey', in: 'header', paramName: 'X-API-Key'},
             {name: 'bearer', type: 'http', scheme: 'bearer'}]};
const values = {'path:id': 'a b', 'query:q': 'x&y', 'header:X-Trace': '1', 'cookie:session': 's', body: '{"a":1}'};
const creds = {key: {value: 's3cret'}, bearer: {value: 'b3arer-xyz'}};
let r = t.buildRequest(op, 'https://api.example.com/v1/', values, creds, false);
assert.strictEqual(r.url, 'https://api.example.com/v1/pets/a%20b?q=x%26y');
assert.deepStrictEqual(r.headers, {'X-Trace': '1', 'X-API-Key': 's3cret', Authorization: 'Bearer b3arer-xyz', 'Content-Type': 'application/json'});
assert.strictEqual(r.body, '{"a":1}');
assert.deepStrictEqual(r.missing, []);
assert.strictEqual(r.notes.length, 1);
const shown = t.curlOf(t.buildRequest(op, 'https://api.example.com/v1', values, creds, true));
assert.ok(!shown.includes('s3cret') && !shown.includes('b3arer-xyz'), shown);
assert.ok(shown.includes("-H 'X-API-Key: <X-API-Key>'") && shown.includes("--data-raw '{\"a\":1}'"), shown);
r = t.buildRequest(op, 'https://api.example.com', {}, {}, false);
assert.deepStrictEqual(r.missing, ['id', 'request body']);
const basic = {method: 'GET', path: '/', security: [{name: 'b', type: 'http', scheme: 'basic'},
  {name: 'k', type: 'apiKey', in: 'query', paramName: 'api_key'}, {name: 'c', type: 'apiKey', in: 'cookie', paramName: 'sid'}]};
r = t.buildRequest(basic, 'https://api.example.com', {}, {b: {user: 'ann', password: 'pä'}, k: {value: 'k1'}, c: {value: 'z'}}, false);
assert.strictEqual(r.headers.Authorization, 'Basic ' + Buffer.from('ann:pä').toString('base64'));
assert.strictEqual(r.url, 'https://api.example.com/?api_key=k1');
assert.strictEqual(r.notes.length, 1);
r = t.buildRequest(basic, 'https://api.example.com', {}, {k: {value: 'k1'}}, true);
assert.strictEqual(r.url, 'https://api.example.com/?api_key=<api_key>');
assert.strictEqual(t.curlOf({method: 'GET', url: "https://x.example.com/it's", headers: {}}), "curl -X GET 'https://x.example.com/it'\\''s'");
assert.strictEqual(t.pretty('{"a":1}', 'application/json'), '{\n  "a": 1\n}');
assert.strictEqual(t.pretty('{bad', 'application/json'), '{bad');
assert.strictEqual(t.pretty('<x>', 'text/html'), '<x>');
const html = p.runnerHTML('textkit', 'https://cdn.example.com/textkit.js', 'console.log("</script><b>")', 'tok');
assert.ok(!html.includes('</script><b>'), 'user code must not close the runner script');
assert.ok(html.includes('"imports":{"textkit":"https://cdn.example.com/textkit.js"}'), html);
console.log('ok');
`
	cmd := exec.Command(node, "-e", check) // #nosec G204 -- test harness
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("node: %v\n%s", err, out)
	}
}
