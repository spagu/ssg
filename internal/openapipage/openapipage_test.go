package openapipage

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/openapi"
)

func load(t *testing.T) *openapi.Spec {
	t.Helper()
	spec, diags, err := openapi.Load("testdata/shop.yaml")
	if err != nil || len(diags) > 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return spec
}

func pages(t *testing.T, tryIt bool) map[string]Page {
	out := map[string]Page{}
	for _, p := range Build(load(t), Options{Base: "/api/shop/", TryIt: tryIt}) {
		out[p.URL] = p
	}
	return out
}

func TestBuildPages(t *testing.T) {
	ps := pages(t, true)
	want := "/api/shop/ /api/shop/empty/ /api/shop/orders/ /api/shop/schemas-tag/ /api/shop/schemas/"
	got := Build(load(t), Options{Base: "/api/shop/"})
	var order []string
	for _, p := range got {
		order = append(order, p.URL)
	}
	if strings.Join(order, " ") != want {
		t.Errorf("urls: %v", order)
	}
	idx := ps["/api/shop/"]
	if idx.Title != "Shop" || idx.Summary != "Orders and products." || idx.Layout != "api-index" {
		t.Errorf("index: %+v", idx)
	}
	for _, s := range []string{
		"**Version** 2.0 · OpenAPI 3.1.0",
		"- `https://{region}.api.example.com/v2` — Regional",
		"  - `{region}` default `eu` (one of `eu`, `us`)",
		"- `/v2`\n",
		"- **key** — API key in the query `api_key`. Issued on signup",
		"- **basic** — HTTP Basic authentication",
		"- **token** — OAuth 2.0 bearer token",
		"- **oidc** — OpenID Connect bearer token",
		"- **mtls** — mutualTLS",
		"- **jwt** — HTTP bearer token — JWT",
		"| <span class=\"ssg-method ssg-method-post\" data-ssg-rest>POST</span> | [`/orders`](/api/shop/orders/#createorder) | Create an order |",
		"**[Empty](/api/shop/empty/)**\n\nNo operations yet.",
		"Data types: [Schemas](/api/shop/schemas/).",
	} {
		if !strings.Contains(idx.Markdown, s) {
			t.Errorf("index lacks %q:\n%s", s, idx.Markdown)
		}
	}
	orders := ps["/api/shop/orders/"]
	if orders.Title != "Orders — Shop" || orders.Summary != "Placing and reading orders." || orders.Layout != "api-module" {
		t.Errorf("orders: %+v", orders)
	}
	for _, s := range []string{
		`<h2 id="createorder">Create an order</h2>`,
		`<span class="ssg-method ssg-method-post" data-ssg-rest>POST</span> <code>/orders</code>`,
		"**Authorization:** [key](/api/shop/#authentication) or none",
		"**Authorization:** [basic](/api/shop/#authentication) or [token](/api/shop/#authentication) (orders:read)",
		"Content type `application/json`: [`Order`](/api/shop/schemas/#schema-order)",
		"| `400` | Bad \\| request | `text/plain` |",
		"**201 example** (`application/json`)\n\n```json\n{\n  \"id\": 7\n}\n```",
		"**400 example** (`text/plain`)\n\n```\nbad input\n```",
		"| `id` (required) | path | `integer` |  |",
		"| `X-Trace` | header | `string` | **Deprecated.** Trace id Default `\"abc\"`. |",
		"| `view` | query | `string` | One of `\"full\"`, `\"short\"`. |",
		`data-ssg-tryit="`,
	} {
		if !strings.Contains(orders.Markdown, s) {
			t.Errorf("orders lacks %q:\n%s", s, orders.Markdown)
		}
	}
	if ping := ps["/api/shop/schemas-tag/"].Markdown; !strings.Contains(ping, "<h2 id=\"get-ping\">GET /ping</h2>") ||
		!strings.Contains(ping, "**Authorization:** none") {
		t.Errorf("ping:\n%s", ping)
	}
	if empty := ps["/api/shop/empty/"].Markdown; empty != "No operations yet.\n\n" {
		t.Errorf("empty tag: %q", empty)
	}
	schemas := ps["/api/shop/schemas/"].Markdown
	for _, s := range []string{
		`<h2 id="schema-order"><code>Order</code></h2>`,
		"| `id` (required) | `integer` | Minimum 1. Maximum 9. Read-only. |",
		"| `note` | `string` | At least 1 characters. At most 80 characters. Pattern `^[a-z]+$`. Write-only. |",
		"| `status` | `string` | **Deprecated.** One of `\"new\"`, `\"paid\"`. |",
		"| `lines[].qty` | `integer` | Default `1`. |",
		"| `customer.address.city` | `object` |  |",
		"| `related` | array of [`Order`](/api/shop/schemas/#schema-order) |  |",
		"Type: `string`\n",
	} {
		if !strings.Contains(schemas, s) {
			t.Errorf("schemas lack %q:\n%s", s, schemas)
		}
	}
	if strings.Contains(schemas, "customer.address.city.zip") {
		t.Error("nesting stops at the depth limit")
	}
	if strings.Contains(pages(t, false)["/api/shop/orders/"].Markdown, "data-ssg-tryit") {
		t.Error("try_it off, no console")
	}
}

var consoleRe = regexp.MustCompile(`data-ssg-tryit="([^"]*)"`)

func TestConsoleData(t *testing.T) {
	m := consoleRe.FindAllStringSubmatch(pages(t, true)["/api/shop/orders/"].Markdown, -1)
	if len(m) != 2 {
		t.Fatalf("consoles: %d", len(m))
	}
	var create, get consoleOp
	if err := json.Unmarshal([]byte(html.UnescapeString(m[0][1])), &create); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1][1])), &get); err != nil {
		t.Fatal(err)
	}
	if create.Method != "POST" || strings.Join(create.Servers, " ") != "https://eu.api.example.com/v2 /v2" ||
		create.Body == nil || !create.Body.Required || !strings.Contains(create.Body.Example, `"customer": {`) ||
		strings.Contains(create.Body.Example, `"id"`) || len(create.Security) != 1 || create.Security[0].ParamName != "api_key" {
		t.Errorf("create: %+v %+v", create, create.Body)
	}
	flags := consoleOf(&openapi.Spec{}, &openapi.Operation{Parameters: []openapi.Parameter{{Name: "on", In: "query", Schema: &openapi.Schema{Type: "boolean"}}}})
	if len(flags.Params[0].Enum) != 2 {
		t.Error("a boolean is a choice")
	}
	if len(get.Params) != 3 || get.Params[0].Example != float64(7) || get.Params[1].Example != "abc" ||
		len(get.Params[2].Enum) != 2 || len(get.Security) != 2 || get.Security[0].Scheme != "basic" {
		t.Errorf("get: %+v", get)
	}
}

func TestSampleValues(t *testing.T) {
	ps := pages(t, true)
	m := consoleRe.FindStringSubmatch(ps["/api/shop/orders/"].Markdown)
	body := html.UnescapeString(m[1])
	for _, s := range []string{`2026-01-31T12:00:00Z`, `2026-01-31`, `user@example.com`, `https://example.com/`,
		`00000000-0000-0000-0000-000000000000`, `\"paid\": false`, `\"total\": 0`, `\"kind\": \"retail\"`,
		`\"empty\": []`, `\"status\": \"new\"`} {
		if !strings.Contains(body, s) {
			t.Errorf("sample lacks %s:\n%s", s, body)
		}
	}
	if sample(nil, nil, 0) != nil || sample(&openapi.Schema{Type: "object"}, nil, 0) != nil {
		t.Error("nothing to sample")
	}
	deep := &openapi.Schema{Ref: "Loop"}
	named := map[string]*openapi.Schema{"Loop": {Type: "array", Items: deep}}
	if v, _ := json.Marshal(sample(deep, named, 0)); !strings.HasPrefix(string(v), "[[") || len(v) > 20 {
		t.Errorf("recursion stops: %s", v)
	}
	if exampleText(openapi.MediaType{Type: "application/json", Example: func() {}}, nil) != "" {
		t.Error("an unencodable example is dropped")
	}
	if exampleText(openapi.MediaType{Type: "application/json"}, nil) != "" {
		t.Error("no schema, no example")
	}
	if jsonText(func() {}) == "" {
		t.Error("jsonText falls back to fmt")
	}
}

func TestNavAndCrumbs(t *testing.T) {
	spec := load(t)
	opts := Options{Base: "/api/shop/"}
	nav := Nav(spec, opts)
	var titles []string
	for _, n := range nav {
		titles = append(titles, n.Title+"="+n.URL)
	}
	if strings.Join(titles, " ") != "Overview=/api/shop/ Orders=/api/shop/orders/ Schemas=/api/shop/schemas-tag/ Empty=/api/shop/empty/ Schemas=/api/shop/schemas/" {
		t.Errorf("nav: %v", titles)
	}
	ps := Build(spec, opts)
	if c := Crumbs(spec, ps[0], opts); len(c) != 1 || c[0].Title != "Shop" {
		t.Errorf("index crumbs: %+v", c)
	}
	if c := Crumbs(spec, ps[2], opts); len(c) != 2 || c[1].Title != "Orders" || c[1].URL != "/api/shop/orders/" {
		t.Errorf("tag crumbs: %+v", c)
	}
	bare := &openapi.Spec{Title: " "}
	if Name(bare) != "REST API" || len(Nav(bare, opts)) != 1 {
		t.Error("an untitled API with no tags")
	}
	if ps := Build(bare, opts); len(ps) != 1 || ps[0].Summary != "REST API REST API reference." {
		t.Errorf("bare: %+v", ps)
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"Get One Pet!": "get-one-pet", "--": "x", "a__b": "a-b", "Ünï": "n"} {
		if Slug(in) != want {
			t.Errorf("Slug(%q) = %q", in, Slug(in))
		}
	}
	if summary("", "  ", "a\nb\n\nc") != "a b" || summary() != "" {
		t.Error("summary")
	}
	w := &writer{site: &site{}}
	if w.typeOf(nil) != "" || describe("d", nil, true) != "**Deprecated.** d" {
		t.Error("nil schema")
	}
	if fence("text/plain", "x") != "```\nx\n```" {
		t.Error("fence")
	}
}

func consoleOf(spec *openapi.Spec, op *openapi.Operation) consoleOp {
	return newSite(spec, Options{}).console(op)
}
