package main

// Credentials on the command line (#132). They reach the engine and nothing
// else: .ssg.yaml is a file people commit, so a password must never land in it.

import (
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/migrate"
)

func TestParseMigrateAuthFlags(t *testing.T) {
	f, code := parseMigrateFlags([]string{
		"--auth-user", "editor", "--auth-pass=s3cret", "--auth-token", "tok",
		"--custom-types", "cpt_services, cpt_team", "--no-custom-types"})
	if code != -1 {
		t.Fatalf("code = %d", code)
	}
	if f.authUser != "editor" || f.authPass != "s3cret" || f.authToken != "tok" {
		t.Fatalf("credentials = %+v", f)
	}
	if len(f.customTypes) != 2 || f.customTypes[1] != "cpt_team" {
		t.Fatalf("custom types = %v", f.customTypes)
	}
	if !f.noCustomTypes {
		t.Fatal("--no-custom-types not parsed")
	}
	// The defaults carry nothing.
	if d, _ := parseMigrateFlags(nil); d.authUser != "" || d.authToken != "" || d.noCustomTypes {
		t.Fatalf("defaults must be empty: %+v", d)
	}
}

// TestMigrateNeverWritesCredentials: the whole point of holding them in the
// flag struct. A password in a committed config outlives the migration.
func TestMigrateNeverWritesCredentials(t *testing.T) {
	t.Chdir(t.TempDir())
	var got migrate.Options
	stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		got = opts
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})

	code := runMigrate([]string{"wordpress", "https://shop.example.com", "--quiet",
		"--auth-user", "editor", "--auth-pass", "s3cret", "--custom-types", "cpt_services"})
	if code != 0 {
		t.Fatalf("migrate = %d", code)
	}
	if got.AuthUser != "editor" || got.AuthPass != "s3cret" {
		t.Fatalf("credentials never reached the engine: %+v", got)
	}
	if len(got.CustomTypes) != 1 || got.CustomTypes[0] != "cpt_services" {
		t.Fatalf("custom types lost: %v", got.CustomTypes)
	}

	cfg, err := os.ReadFile(".ssg.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"s3cret", "editor", "auth_user", "auth-pass"} {
		if strings.Contains(string(cfg), secret) {
			t.Fatalf("%q leaked into the config:\n%s", secret, cfg)
		}
	}
}

// TestMigrateUsageMentionsAuth: an operator whose menus are missing has to be
// able to find the flag that fixes it.
func TestMigrateUsageMentionsAuth(t *testing.T) {
	out, err := captureStdout(func() error {
		printMigrateUsage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--auth-user", "--auth-token", "--custom-types"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage does not mention %s:\n%s", want, out)
		}
	}
}
