package openapi

import (
	"strings"
	"testing"
)

// head is the minimal valid prefix shared by the diagnostic test documents.
const head = "openapi: 3.0.0\ninfo: {title: t, version: '1'}\n"

func TestDiagnostics(t *testing.T) {
	cases := []struct {
		name, doc, path, msg string
	}{
		{"missingPathParam", head + `
paths:
  /a/{id}:
    get: {responses: {'200': {description: ok}}}
`, "#/paths/~1a~1{id}/get/parameters", `path parameter "id"`},
		{"noResponses", head + "paths:\n  /a:\n    get: {summary: x}\n", "#/paths/~1a/get/responses", "no responses"},
		{"unknownScheme", head + "security:\n  - nope: []\n", "#/security/0", `unknown security scheme "nope"`},
		{"duplicateID", head + `
paths:
  /a:
    get: {operationId: same, responses: {'200': {description: ok}}}
    put: {operationId: same, responses: {'200': {description: ok}}}
`, "#/paths/~1a/put/operationId", `duplicate operationId "same"`},
		{"unknownSchema", head + "components:\n  schemas:\n    A: {$ref: '#/components/schemas/B'}\n", "#/components/schemas/A", "unknown schema"},
		{"externalRef", head + `
paths:
  /a:
    get:
      responses:
        '200': {$ref: 'other.yaml#/r'}
`, "#/paths/~1a/get/responses/200", "external $ref not supported"},
		{"externalSchemaRef", head + "components:\n  schemas:\n    A: {$ref: 'https://example.com/a.json'}\n", "#/components/schemas/A", "external $ref"},
		{"unresolvedRef", head + "paths:\n  /a:\n    $ref: '#/nowhere'\n", "#/paths/~1a", "unresolved $ref"},
		{"refCycle", head + `
paths:
  /a:
    get:
      parameters: [{$ref: '#/components/parameters/p'}]
      responses: {'200': {description: ok}}
components:
  parameters:
    p: {$ref: '#/components/parameters/q'}
    q: {$ref: '#/components/parameters/p'}
`, "#/paths/~1a/get/parameters/0", "cycle"},
		{"serverWithoutURL", head + "servers:\n  - description: none\n", "#/servers/0", "server without url"},
		{"deepSchema", head + "components:\n  schemas:\n    A: " + strings.Repeat("{items: ", 40) + "{}" + strings.Repeat("}", 40) + "\n",
			"#/components/schemas/A" + strings.Repeat("/items", 33), "nested deeper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := mustParse(t, tc.doc)
			for _, d := range diags {
				if d.Path == tc.path && strings.Contains(d.Message, tc.msg) {
					if !strings.HasPrefix(d.String(), tc.path+": ") {
						t.Errorf("String() = %q", d.String())
					}
					return
				}
			}
			t.Errorf("want %s: %s, got %v", tc.path, tc.msg, diags)
		})
	}
}

func TestOperationIDs(t *testing.T) {
	spec, diags := mustParse(t, head+`
paths:
  /a/{b}.json:
    parameters: [{name: b, in: path, required: true}]
    get: {responses: {'200': {description: ok}}}
    put: {operationId: get-a-b-json, responses: {'200': {description: ok}}}
    post: {operationId: get-a-b-json, responses: {'200': {description: ok}}}
  /:
    get: {responses: {'200': {description: ok}}}
`)
	var ids []string
	for _, op := range spec.Operations {
		ids = append(ids, op.ID)
	}
	if got := strings.Join(ids, ","); got != "get-a-b-json,get-a-b-json-2,get-a-b-json-3,get" {
		t.Errorf("ids = %s", got)
	}
	if len(diags) != 2 {
		t.Errorf("want 2 duplicate diagnostics (explicit only), got %v", diags)
	}
}

func TestTagsDeclaredDefault(t *testing.T) {
	spec, _ := mustParse(t, head+`
tags: [{name: default, description: Misc}, {name: ''}, {name: b}, {name: b}]
paths:
  /a:
    get: {responses: {'200': {description: ok}}}
    put: {tags: [c, b], responses: {'200': {description: ok}}}
`)
	want := []Tag{{"default", "Misc"}, {"b", ""}, {"c", ""}}
	if len(spec.Tags) != 3 || spec.Tags[0] != want[0] || spec.Tags[1] != want[1] || spec.Tags[2] != want[2] {
		t.Errorf("tags = %+v", spec.Tags)
	}
}

func TestSkippedEntries(t *testing.T) {
	spec, diags := mustParse(t, head+`
x-top: 1
paths:
  x-ext: {}
  /a:
    get:
      parameters:
        - {$ref: '#/missing'}
        - name: q
          in: query
          content:
            application/json:
              schema: {type: object}
              examples:
                first: {$ref: '#/components/examples/e'}
      requestBody: {$ref: '#/missing'}
      responses:
        x-ext: {}
        '200':
          description: ok
          headers:
            X-Bad: {$ref: '#/missing'}
components:
  examples:
    e: {value: {k: [1, true, null]}}
  schemas:
    x-ext: {}
  securitySchemes:
    x-ext: {}
    s: {$ref: '#/missing'}
`)
	op := spec.Operations[0]
	if len(spec.Operations) != 1 || len(op.Parameters) != 1 || op.RequestBody != nil || len(op.Responses) != 1 || op.Responses[0].Headers != nil {
		t.Errorf("op = %+v", op)
	}
	if op.Parameters[0].Schema.Type != "object" {
		t.Errorf("content schema = %+v", op.Parameters[0])
	}
	if spec.Schemas != nil || spec.SecuritySchemes != nil {
		t.Errorf("components = %+v %+v", spec.Schemas, spec.SecuritySchemes)
	}
	if len(diags) != 4 {
		t.Errorf("diags = %v", diags)
	}
}
