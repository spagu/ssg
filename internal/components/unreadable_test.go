package components

// A component file that exists but cannot be read is an error, not a skip:
// "no template" is a stray folder, "cannot read the template" is a broken one.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnreadableTemplateFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "card", templateFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(dir, nil); err == nil {
		t.Error("a template that is a directory must fail the load")
	}
}

func TestUnreadableSchemaFails(t *testing.T) {
	dir := t.TempDir()
	card := filepath.Join(dir, "card")
	if err := os.MkdirAll(filepath.Join(card, schemaFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(card, templateFile), []byte("<div></div>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(dir, nil); err == nil {
		t.Error("a schema that is a directory must fail the load")
	}
}
