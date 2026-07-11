package deploy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// newEd25519Signer returns an SSH signer for a fresh ed25519 key.
func newEd25519Signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// newClientKeyFile writes a fresh ed25519 private key in OpenSSH PEM form and returns
// its path plus the matching signer.
func newClientKeyFile(t *testing.T) (string, ssh.Signer) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return path, signer
}

// serveOneSFTP accepts a single SSH connection and serves the SFTP subsystem, allowing
// only the given client public key.
func serveOneSFTP(t *testing.T, ln net.Listener, hostKey ssh.Signer, clientPub ssh.PublicKey) {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), clientPub.Marshal()) {
				return nil, nil
			}
			return nil, errUnknownKey
		},
	}
	cfg.AddHostKey(hostKey)

	conn, err := ln.Accept()
	if err != nil {
		return // listener closed at test end
	}
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range chReqs {
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
				_ = req.Reply(ok, nil)
			}
		}()
		srv, err := sftp.NewServer(ch)
		if err != nil {
			return
		}
		_ = srv.Serve()
		return
	}
}

var errUnknownKey = &sshError{"unknown client key"}

type sshError struct{ s string }

func (e *sshError) Error() string { return e.s }

// TestDeploySFTPRoundTrip exercises the full SFTP publish path against an in-process
// SSH/SFTP server: key auth, known_hosts verification, and file upload.
func TestDeploySFTPRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	hostKey := newEd25519Signer(t)
	keyFile, clientSigner := newClientKeyFile(t)
	go serveOneSFTP(t, ln, hostKey, clientSigner.PublicKey())

	// known_hosts entry for the ephemeral listener address.
	khLine := knownhosts.Line([]string{ln.Addr().String()}, hostKey.PublicKey())
	khFile := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(khFile, []byte(khLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	remoteDir := t.TempDir()
	env := map[string]string{"SSH_KEY_FILE": keyFile, "SSH_KNOWN_HOSTS": khFile}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	url, err := deploySFTP(ctx, Options{
		Dir:    writeSite(t),
		Target: "sftp://tester@" + ln.Addr().String() + remoteDir,
		Quiet:  true,
		Env:    func(k string) string { return env[k] },
	})
	if err != nil {
		t.Fatalf("deploySFTP round-trip: %v", err)
	}
	if url == "" {
		t.Error("expected a non-empty sftp URL")
	}
	// The site's index.html must have landed under remoteDir.
	if _, err := os.Stat(filepath.Join(remoteDir, "index.html")); err != nil {
		t.Errorf("uploaded index.html missing on server: %v", err)
	}
}

func TestGitOriginURL(t *testing.T) {
	// Runs git in the current dir (this repo has an origin); must not panic and
	// returns a string (empty is acceptable in a detached/no-remote checkout).
	_ = gitOriginURL(context.Background())
}
