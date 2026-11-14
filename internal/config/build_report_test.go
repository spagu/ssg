package config

import (
	"os"
	"path/filepath"
	"testing"
)

// #421: `build_report: true` wrote a file named "true". A boolean is the
// switch the bare --build-report flag is; any other value is the file name.
func TestBuildReportBooleansAreTheSwitch(t *testing.T) {
	cases := map[string]string{
		"true":                 "build.json",
		"yes":                  "build.json",
		"On":                   "build.json",
		"false":                "",
		"no":                   "",
		"off":                  "",
		`"reports/build.json"`: "reports/build.json",
		`"true.json"`:          "true.json",
	}
	for in, want := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, ".ssg.yaml")
		if err := os.WriteFile(path, []byte("source: content\nbuild_report: "+in+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if cfg.BuildReport != want {
			t.Errorf("build_report: %s → %q, want %q", in, cfg.BuildReport, want)
		}
	}
}
