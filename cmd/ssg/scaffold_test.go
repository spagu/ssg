package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvailableWorkerTemplates(t *testing.T) {
	names := availableWorkerTemplates()
	want := map[string]bool{"contact-form": false, "stripe-checkout": false, "dynamic-price": false, "conversions-proxy": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("embedded worker template %q missing from %v", name, names)
		}
	}
}

func TestExtractWorkerTemplate(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "contact-form")
	if err := extractWorkerTemplate("workers/contact-form", dest); err != nil {
		t.Fatalf("extractWorkerTemplate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "functions", "api", "contact.ts")); err != nil {
		t.Fatalf("expected the function file to be extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("expected the README to be extracted: %v", err)
	}
}

func TestRunNewWorker_UnknownTemplate(t *testing.T) {
	if code := runNewWorker([]string{"does-not-exist"}); code != 1 {
		t.Fatalf("unknown template should exit 1, got %d", code)
	}
}

func TestRunNewWorker_NoArgs(t *testing.T) {
	if code := runNewWorker(nil); code != 2 {
		t.Fatalf("missing template should exit 2, got %d", code)
	}
}

func TestRunNewWorker_ScaffoldsIntoCwd(t *testing.T) {
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if code := runNewWorker([]string{"dynamic-price"}); code != 0 {
		t.Fatalf("scaffold should succeed, got exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(tmp, "workers", "dynamic-price", "README.md")); err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
	// A second run must refuse to overwrite.
	if code := runNewWorker([]string{"dynamic-price"}); code != 1 {
		t.Fatalf("re-scaffold should refuse, got exit %d", code)
	}
}

func TestWorkerConfigSnippet(t *testing.T) {
	snip := workerConfigSnippet("contact-form")
	if !strings.Contains(snip, "dir: workers/contact-form") || !strings.Contains(snip, "routes_include") {
		t.Fatalf("snippet missing worker config:\n%s", snip)
	}
}
