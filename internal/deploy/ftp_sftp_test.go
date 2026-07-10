package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func noEnv(string) string { return "" }

func TestDeployFTPBadTarget(t *testing.T) {
	dir := writeSite(t)
	// Missing target and wrong scheme both fail before any network use.
	if _, err := deployFTP(context.Background(), Options{Dir: dir, Env: noEnv}); err == nil {
		t.Error("empty ftp target should error")
	}
	if _, err := deployFTP(context.Background(), Options{Dir: dir, Target: "http://h/p", Env: noEnv}); err == nil {
		t.Error("non-ftp scheme should error")
	}
}

func TestDeploySFTPValidation(t *testing.T) {
	dir := writeSite(t)
	// Missing target.
	if _, err := deploySFTP(context.Background(), Options{Dir: dir, Env: noEnv}); err == nil {
		t.Error("empty sftp target should error")
	}
	// Target without a username and no SSH_USERNAME.
	if _, err := deploySFTP(context.Background(), Options{Dir: dir, Target: "sftp://host/p", Env: noEnv}); err == nil {
		t.Error("missing username should error")
	}
}

func TestSSHAuthMethodsPassword(t *testing.T) {
	methods, err := sshAuthMethods(Options{Env: noEnv}, "secret")
	if err != nil || len(methods) != 1 {
		t.Fatalf("password auth = %v, %v", methods, err)
	}
}

func TestSSHAuthMethodsMissingKey(t *testing.T) {
	env := func(k string) string {
		if k == "SSH_KEY_FILE" {
			return filepath.Join(t.TempDir(), "nope_id_rsa")
		}
		return ""
	}
	if _, err := sshAuthMethods(Options{Env: env}, ""); err == nil {
		t.Error("missing key file should error")
	}
}

func TestKnownHostsCallback(t *testing.T) {
	// An empty (but present) known_hosts file yields a usable callback.
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == "SSH_KNOWN_HOSTS" {
			return kh
		}
		return ""
	}
	if cb, err := knownHostsCallback(Options{Env: env}); err != nil || cb == nil {
		t.Fatalf("knownHostsCallback = %v, %v", cb, err)
	}
	// A missing known_hosts file is a hard error (no blind trust).
	miss := func(k string) string {
		if k == "SSH_KNOWN_HOSTS" {
			return filepath.Join(t.TempDir(), "absent")
		}
		return ""
	}
	if _, err := knownHostsCallback(Options{Env: miss}); err == nil {
		t.Error("missing known_hosts should error")
	}
}
