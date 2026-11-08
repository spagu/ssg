package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestWriteLicense: the terms name the license, the condition, and where the
// text that counts lives.
func TestWriteLicense(t *testing.T) {
	var b bytes.Buffer
	writeLicense(&b)
	for _, want := range []string{
		"Business Source License 1.1 (BUSL-1.1)", "Built with ssg", "BSD-3-Clause", licenseURL, licensingURL,
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("license text lacks %q:\n%s", want, b.String())
		}
	}
	if !strings.Contains(licenseLine(), "BUSL-1.1") || !strings.Contains(licenseLine(), "ssg --license") {
		t.Errorf("licenseLine = %q", licenseLine())
	}
}

// TestLicenseFlag: --license prints the terms and exits 0.
func TestLicenseFlag(t *testing.T) {
	if os.Getenv("TEST_LICENSE_FLAG") == "1" {
		parseSpecialFlags("--license")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLicenseFlag") // #nosec G204,G702 -- re-runs this test binary
	cmd.Env = append(os.Environ(), "TEST_LICENSE_FLAG=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "is licensed under the Business Source License 1.1") {
		t.Errorf("err=%v out=%s", err, out)
	}
}

// TestTheLicenseIsNamedWithTheVersion: the help text and the build log's
// first line carry it too.
func TestTheLicenseIsNamedWithTheVersion(t *testing.T) {
	help, _ := captureStdout(func() error { printUsage(); return nil })
	if !strings.Contains(help, licenseLine()) || !strings.Contains(help, "--license") {
		t.Errorf("help lacks the license:\n%s", help[:min(len(help), 400)])
	}
	log, _ := captureStdout(func() error { logRunningVersion(&config.Config{}); return nil })
	if !strings.HasPrefix(log, "🧱 ssg ") || !strings.Contains(log, "· BUSL-1.1") {
		t.Errorf("build log line = %q", log)
	}
}
