package externalsource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDisabledAndEmpty(t *testing.T) {
	reg, warns, err := Load(Config{})
	if err != nil || len(warns) != 0 || len(reg.Data()) != 0 || len(reg.Meta()) != 0 {
		t.Fatalf("disabled load: %+v %v %v", reg, warns, err)
	}
}

func TestLoadFileSources(t *testing.T) {
	tmp := t.TempDir()
	yamlPath := writeFile(t, filepath.Join(tmp, "nav.yaml"), "links:\n  - Home\n  - Blog\n")
	csvPath := writeFile(t, filepath.Join(tmp, "rates.csv"), "code,rate\nPLN,4.30\nGBP,0.84\n")
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"navigation": {Type: "file", Path: yamlPath},
		"rates":      {Type: "file", Path: csvPath},
	}}
	reg, warns, err := Load(cfg)
	if err != nil || len(warns) != 0 {
		t.Fatalf("load: %v %v", err, warns)
	}
	if len(reg.Order) != 2 || reg.Order[0] != "navigation" || reg.Order[1] != "rates" {
		t.Fatalf("order = %v", reg.Order)
	}
	data := reg.Data()
	nav := data["navigation"].(map[string]interface{})
	if nav["links"].([]interface{})[1] != "Blog" {
		t.Fatalf("navigation = %#v", nav)
	}
	meta := reg.Meta()["rates"]
	if meta.SourceType != "file" || meta.ContentType != "csv" || meta.RecordCount != 2 ||
		meta.Checksum == "" || meta.FetchedAt.IsZero() || meta.FromCache || meta.Stale {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestLoadRequiredFailureAbortsBuild(t *testing.T) {
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"missing": {Type: "file", Path: "/nonexistent/file.yaml"},
	}}
	_, _, err := Load(cfg)
	var srcErr *SourceError
	if err == nil || !errors.As(err, &srcErr) {
		t.Fatalf("err = %v", err)
	}
	if srcErr.Source != "missing" || srcErr.SourceType != "file" || srcErr.Stage != "read" {
		t.Fatalf("unified error = %+v", srcErr)
	}
	if !strings.Contains(err.Error(), `external source "missing" (file) failed at read`) {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestLoadOptionalFailureWarns(t *testing.T) {
	tmp := t.TempDir()
	good := writeFile(t, filepath.Join(tmp, "ok.json"), `{"a":1}`)
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"broken": {Type: "file", Path: filepath.Join(tmp, "gone.yaml"), Required: boolPtr(false)},
		"ok":     {Type: "file", Path: good},
	}}
	reg, warns, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "optional") || !strings.Contains(warns[0], "broken") {
		t.Fatalf("warnings = %v", warns)
	}
	if _, present := reg.Data()["broken"]; present {
		t.Fatal("failed optional source must be absent")
	}
	if reg.Data()["ok"] == nil {
		t.Fatal("healthy source must load")
	}
}

func TestLoadInvalidConfigFails(t *testing.T) {
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{"api": {Type: "http"}}}
	if _, _, err := Load(cfg); err == nil {
		t.Fatal("invalid config must fail")
	}
}

func TestFileConnectorLimitsAndStages(t *testing.T) {
	tmp := t.TempDir()
	big := writeFile(t, filepath.Join(tmp, "big.json"), `{"data":"`+strings.Repeat("x", 100)+`"}`)
	src := Source{Name: "big", Type: "file", Format: "json", Path: big, MaxSize: 10}
	_, err := FileConnector{}.Load(src)
	var srcErr *SourceError
	if !errors.As(err, &srcErr) || srcErr.Stage != "read" || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("size limit err = %v", err)
	}
	// Parse stage.
	badFmt := writeFile(t, filepath.Join(tmp, "bad.json"), "{broken")
	src = Source{Name: "bad", Type: "file", Format: "json", Path: badFmt, MaxSize: defaultMaxSize}
	_, err = FileConnector{}.Load(src)
	if !errors.As(err, &srcErr) || srcErr.Stage != "parse" {
		t.Fatalf("parse stage err = %v", err)
	}
	// Transform stage + select success.
	sel := writeFile(t, filepath.Join(tmp, "sel.json"), `{"data":{"items":[1,2]}}`)
	src = Source{Name: "sel", Type: "file", Format: "json", Path: sel, MaxSize: defaultMaxSize,
		Transform: TransformConfig{Select: "data.items"}}
	res, err := FileConnector{}.Load(src)
	if err != nil || res.Metadata.RecordCount != 2 {
		t.Fatalf("select result = %+v, %v", res, err)
	}
	src.Transform.Select = "data.nope"
	_, err = FileConnector{}.Load(src)
	if !errors.As(err, &srcErr) || srcErr.Stage != "transform" {
		t.Fatalf("transform stage err = %v", err)
	}
}
