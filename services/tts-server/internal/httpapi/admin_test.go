package httpapi

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const adminAuth = "Bearer admin"

func (hs *harness) upload(parts [][3]string) (int, string) {
	body, ctype := multipartBody(hs.t, parts)
	req := httptest.NewRequest("POST", "/v1/voices", body)
	rec := hs.send(req, "Content-Type", ctype, "Authorization", adminAuth)
	return rec.Code, rec.Body.String()
}

func TestUploadAndDelete(t *testing.T) {
	hs := newHarness(t)
	code, body := hs.upload([][3]string{
		{"model", "pl_PL-new-low.onnx", "\x08model"}, {"config", "x.json", piperConfig},
	})
	if code != 201 || !strings.Contains(body, `"id":"pl_PL-new-low"`) || !strings.Contains(body, `"default":false`) {
		t.Fatal(code, body)
	}
	code, body = hs.upload([][3]string{
		{"id", "", "custom"}, {"model", "whatever.onnx", "\x08m"}, {"config", "c.json", piperConfig},
	})
	if code != 201 || !strings.Contains(body, `"id":"custom"`) {
		t.Fatal(code, body)
	}
	if code, _ := hs.upload([][3]string{{"model", "custom.onnx", "\x08m"}, {"config", "c", piperConfig}}); code != 409 {
		t.Fatal("duplicate must conflict", code)
	}
	if rec := hs.do("DELETE", "/v1/voices/custom", "", "Authorization", adminAuth); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("DELETE", "/v1/voices/custom", "", "Authorization", adminAuth); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("DELETE", "/v1/voices/espeak:en", "", "Authorization", adminAuth); rec.Code != 400 {
		t.Fatal("built-in delete", rec.Code)
	}
	entries, _ := os.ReadDir(hs.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			t.Fatal("staged file left behind:", e.Name())
		}
	}
}

func TestUploadErrors(t *testing.T) {
	hs := newHarness(t)
	model := [3]string{"model", "m.onnx", "\x08m"}
	config := [3]string{"config", "c.json", piperConfig}
	cases := map[string]struct {
		parts [][3]string
		code  int
	}{
		"missing config": {[][3]string{model}, 400},
		"bad id":         {[][3]string{{"id", "", "../x"}, model, config}, 400},
		"bad config":     {[][3]string{model, {"config", "c", "{}"}}, 400},
		"not onnx":       {[][3]string{{"model", "m.onnx", "PK"}, config}, 400},
		"empty model":    {[][3]string{{"model", "m.onnx", ""}, config}, 400},
		"twice model":    {[][3]string{model, model, config}, 400},
		"twice config":   {[][3]string{config, config, model}, 400},
		"unknown field":  {[][3]string{{"extra", "", "x"}}, 400},
		"model too big":  {[][3]string{{"model", "m.onnx", strings.Repeat("x", 1025)}, config}, 413},
	}
	for name, c := range cases {
		if code, body := hs.upload(c.parts); code != c.code {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	req := httptest.NewRequest("POST", "/v1/voices", strings.NewReader("x"))
	if rec := hs.send(req, "Authorization", adminAuth); rec.Code != 400 {
		t.Fatal("non-multipart", rec.Code)
	}
	req = httptest.NewRequest("POST", "/v1/voices", strings.NewReader("--b\r\nbroken"))
	if rec := hs.send(req, "Authorization", adminAuth, "Content-Type", "multipart/form-data; boundary=b"); rec.Code != 400 {
		t.Fatal("broken multipart", rec.Code)
	}
	hs.piper = false
	if code, _ := hs.upload([][3]string{model, config}); code != 503 {
		t.Fatal("piper missing", code)
	}
}

func TestUploadBodyLimit(t *testing.T) {
	hs := newHarness(t, func(d *Deps) { d.MaxUploadBytes = 1 })
	big := strings.Repeat("y", 5<<20)
	if code, _ := hs.upload([][3]string{{"config", "c", big}}); code != 413 {
		t.Fatal(code)
	}
}

func TestDeleteReloadFailure(t *testing.T) {
	hs := newHarness(t)
	// voices.yaml referencing the voice makes the post-delete reload fail
	_ = os.WriteFile(hs.dir+"/voices.yaml", []byte("defaults: {pl: gosia}\n"), 0o600)
	if rec := hs.do("DELETE", "/v1/voices/gosia", "", "Authorization", adminAuth); rec.Code != 500 {
		t.Fatal(rec.Code)
	}
	// the files are gone but the old catalog still lists the voice
	if rec := hs.do("DELETE", "/v1/voices/gosia", "", "Authorization", adminAuth); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
