package openapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSmall31JSON(t *testing.T) {
	spec, diags, err := Load(filepath.Join("testdata", "small31.json"))
	if err != nil || len(diags) != 0 {
		t.Fatalf("Load: %v %v", err, diags)
	}
	if spec.Version != "3.1.0" || spec.Summary != "A 3.1 sample" || spec.Security != nil {
		t.Errorf("spec = %+v", spec)
	}
	op := spec.Operations[0]
	if op.ID != "get-items-id" || op.Parameters[0].Schema.Type != "string | null" || op.Parameters[0].Schema.Example != "abc" {
		t.Errorf("op = %+v / %+v", op, op.Parameters[0].Schema)
	}
	item := spec.Schemas[0].Schema
	if item.AdditionalProperties == nil || item.Properties[0].Schema.Type != "string | null" || item.Properties[1].Schema.TypeString() != "array of string" {
		t.Errorf("item = %+v", item)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"empty":     "",
		"notYAML":   "openapi: [unclosed",
		"notObject": "- a\n- b\n",
		"noVersion": "info: {title: x}\n",
		"swagger":   "swagger: '2.0'\n",
		"version2":  "openapi: 2.0.0\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Load(path); err == nil {
				t.Errorf("expected error for %q", doc)
			}
		})
	}
	if _, _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
}
