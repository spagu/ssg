package openapi

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mustParse parses doc and fails the test on a hard error.
func mustParse(t *testing.T, doc string) (*Spec, []Diagnostic) {
	t.Helper()
	spec, diags, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return spec, diags
}

// loadPetstore loads testdata/petstore.yaml and expects no diagnostics.
func loadPetstore(t *testing.T) *Spec {
	t.Helper()
	spec, diags, err := Load(filepath.Join("testdata", "petstore.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	return spec
}

func TestPetstoreInfoAndServers(t *testing.T) {
	spec := loadPetstore(t)
	if spec.Version != "3.0.3" || spec.Title != "Petstore" || spec.APIVersion != "1.2.0" || spec.Description == "" {
		t.Errorf("info = %+v", spec)
	}
	want := []Server{{
		URL:         "https://api.example.com/{version}",
		Description: "Production",
		Variables:   map[string]ServerVariable{"version": {Default: "v1", Enum: []string{"v1", "v2"}, Description: "API version"}},
	}}
	if !reflect.DeepEqual(spec.Servers, want) {
		t.Errorf("servers = %+v", spec.Servers)
	}
}

func TestPetstoreOrderAndTags(t *testing.T) {
	spec := loadPetstore(t)
	var got []string
	for _, op := range spec.Operations {
		got = append(got, op.ID+" "+op.Method+" "+op.Path)
	}
	want := []string{"listPets GET /pets", "post-pets POST /pets", "get-pets-petId GET /pets/{petId}", "delete-pets-petId DELETE /pets/{petId}"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("operations = %q", got)
	}
	wantTags := []Tag{{"pets", "Everything about pets"}, {"store", ""}, {"admin", ""}, {"default", ""}}
	if !reflect.DeepEqual(spec.Tags, wantTags) {
		t.Errorf("tags = %+v", spec.Tags)
	}
	if !reflect.DeepEqual(spec.Operations[2].Tags, []string{"default"}) {
		t.Errorf("untagged op tags = %v", spec.Operations[2].Tags)
	}
}

func TestPetstoreListPets(t *testing.T) {
	op := loadPetstore(t).Operations[0]
	if len(op.Parameters) != 2 || op.Parameters[0].Name != "limit" || op.Parameters[0].Example != 20 {
		t.Fatalf("params = %+v", op.Parameters)
	}
	if s := op.Parameters[0].Schema; *s.Minimum != 1 || *s.Maximum != 100 {
		t.Errorf("limit schema = %+v", s)
	}
	if s := op.Parameters[1].Schema; s.Default != "available" || !reflect.DeepEqual(s.Enum, []any{"available", "sold"}) {
		t.Errorf("status schema = %+v", s)
	}
	if len(op.Responses) != 2 || op.Responses[0].Status != "200" || op.Responses[1].Status != "default" {
		t.Fatalf("responses = %+v", op.Responses)
	}
	ok := op.Responses[0]
	if !reflect.DeepEqual(ok.Headers, []Header{{Name: "X-Next", Description: "Link to the next page", Schema: &Schema{Type: "string"}}}) {
		t.Errorf("headers = %+v", ok.Headers)
	}
	mt := ok.Content[0]
	if mt.Type != "application/json" || mt.Schema.TypeString() != "array of Pet" {
		t.Errorf("media = %+v", mt)
	}
	wantEx := []any{map[string]any{"id": 1, "name": "Rex"}, map[string]any{"id": 2, "name": "Tom"}}
	if !reflect.DeepEqual(mt.Example, wantEx) {
		t.Errorf("example = %#v", mt.Example)
	}
	if op.Responses[1].Description != "Unexpected error" || op.Responses[1].Content[0].Schema.Ref != "Error" {
		t.Errorf("default response = %+v", op.Responses[1])
	}
	if !reflect.DeepEqual(op.Security, []Requirement{{{Scheme: "apiKey"}}}) || op.Servers != nil {
		t.Errorf("security/servers = %+v %+v", op.Security, op.Servers)
	}
}

func TestPetstoreOverrides(t *testing.T) {
	ops := loadPetstore(t).Operations
	post, get, del := ops[1], ops[2], ops[3]
	wantSec := []Requirement{{{Scheme: "bearer", Scopes: []string{"write"}}, {Scheme: "apiKey"}}}
	if !reflect.DeepEqual(post.Security, wantSec) {
		t.Errorf("post security = %+v", post.Security)
	}
	if rb := post.RequestBody; rb == nil || !rb.Required || rb.Description != "The pet to add" || rb.Content[0].Schema.Ref != "Pet" {
		t.Errorf("request body = %+v", rb)
	}
	if len(get.Parameters) != 1 || get.Parameters[0].Description != "Operation description" || get.Parameters[0].Schema.TypeString() != "integer" {
		t.Errorf("merged params = %+v", get.Parameters)
	}
	if get.Servers[0].URL != "https://pets.example.com" {
		t.Errorf("path servers = %+v", get.Servers)
	}
	wantEx := map[string]any{"id": 1, "name": "Rex", "tags": []any{"good"}}
	if !reflect.DeepEqual(get.Responses[0].Content[0].Example, wantEx) {
		t.Errorf("example = %#v", get.Responses[0].Content[0].Example)
	}
	if del.Security == nil || len(del.Security) != 0 || !del.Deprecated || del.Servers[0].URL != "https://admin.example.com" {
		t.Errorf("delete = %+v", del)
	}
	if del.Parameters[0].Description != "Shared description" || del.Parameters[0].Schema.TypeString() != "integer (int64)" {
		t.Errorf("inherited params = %+v", del.Parameters)
	}
}

func TestPetstoreComponents(t *testing.T) {
	spec := loadPetstore(t)
	wantSchemes := []SecurityScheme{
		{Name: "apiKey", Type: "apiKey", Description: "Key issued in the dashboard", In: "header", ParamName: "X-API-Key"},
		{Name: "bearer", Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	if !reflect.DeepEqual(spec.SecuritySchemes, wantSchemes) {
		t.Errorf("schemes = %+v", spec.SecuritySchemes)
	}
	var names []string
	for _, s := range spec.Schemas {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Pet,Person,Company,Error" {
		t.Errorf("schemas = %v", names)
	}
	pet := spec.Schemas[0].Schema
	types := map[string]string{}
	for _, p := range pet.Properties {
		types[p.Name] = p.Schema.TypeString()
	}
	want := map[string]string{
		"id": "integer (int64)", "name": "string", "nickname": "string | null", "status": "string",
		"parent": "Pet", "owner": "oneOf Person | Company", "attributes": "object",
	}
	if !reflect.DeepEqual(types, want) {
		t.Errorf("property types = %v", types)
	}
	if !pet.Properties[0].Required || pet.Properties[2].Required || !pet.Properties[0].Schema.ReadOnly {
		t.Errorf("required/readOnly wrong: %+v", pet.Properties)
	}
	name := pet.Properties[1].Schema
	if *name.MinLength != 1 || *name.MaxLength != 64 || name.Pattern != "^[A-Za-z]+$" {
		t.Errorf("name = %+v", name)
	}
	if pet.Properties[6].Schema.AdditionalProperties.Type != "string" {
		t.Errorf("additionalProperties = %+v", pet.Properties[6].Schema)
	}
	if !spec.Schemas[2].Schema.Properties[0].Schema.Deprecated || !spec.Schemas[3].Schema.Properties[0].Schema.WriteOnly {
		t.Error("deprecated/writeOnly not read")
	}
}
