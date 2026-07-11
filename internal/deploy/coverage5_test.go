package deploy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// TestNetlifySSLURLFallback covers the ssl_url→url fallback in netlifyCreateDeploy.
func TestNetlifySSLURLFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"d","required":[],"url":"http://plain"}`)) // no ssl_url
	}))
	t.Cleanup(srv.Close)
	old := netlifyAPIBase
	netlifyAPIBase = srv.URL
	t.Cleanup(func() { netlifyAPIBase = old })
	url, err := deployNetlify(context.Background(), Options{
		Dir: writeSite(t), Project: "s", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "t"}[k] },
	})
	if err != nil || url != "http://plain" {
		t.Errorf("fallback url = %q, %v", url, err)
	}
}

// TestVercelWithTeamID covers the teamId query-string branches (upload + deploy).
func TestVercelWithTeamID(t *testing.T) {
	sawTeam := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("teamId") == "org1" {
			sawTeam = true
		}
		if r.URL.Path == "/v2/files" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{"url":"proj.vercel.app"}`))
	}))
	t.Cleanup(srv.Close)
	old := vercelAPIBase
	vercelAPIBase = srv.URL
	t.Cleanup(func() { vercelAPIBase = old })
	if _, err := deployVercel(context.Background(), Options{
		Dir: writeSite(t), Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t", "VERCEL_ORG_ID": "org1"}[k] },
	}); err != nil {
		t.Fatalf("deployVercel with team: %v", err)
	}
	if !sawTeam {
		t.Error("teamId query parameter was not sent")
	}
}

// TestVercelEmptyDeploymentURL covers the empty-url return branch.
func TestVercelEmptyDeploymentURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/files" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{"url":""}`)) // no URL reported
	}))
	t.Cleanup(srv.Close)
	old := vercelAPIBase
	vercelAPIBase = srv.URL
	t.Cleanup(func() { vercelAPIBase = old })
	url, err := deployVercel(context.Background(), Options{
		Dir: writeSite(t), Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t"}[k] },
	})
	if err != nil || url != "" {
		t.Errorf("empty deployment url = %q, %v", url, err)
	}
}

// serveSSHRejectSFTP authenticates and opens a session but rejects the sftp
// subsystem, so sftp.NewClient fails.
func serveSSHRejectSFTP(t *testing.T, ln net.Listener, hostKey ssh.Signer, clientPub ssh.PublicKey) {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_ = clientPub
	cfg.AddHostKey(hostKey)
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			return
		}
		for req := range chReqs {
			_ = req.Reply(false, nil) // reject subsystem sftp
		}
		_ = ch.Close()
		return
	}
}

func sftpTestKnownHosts(t *testing.T, addr string, hostKey ssh.Signer) string {
	t.Helper()
	line := knownhosts.Line([]string{addr}, hostKey.PublicKey())
	p := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(p, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDeploySFTPNewClientError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ln.Close() }()
	hostKey := newEd25519Signer(t)
	keyFile, signer := newClientKeyFile(t)
	go serveSSHRejectSFTP(t, ln, hostKey, signer.PublicKey())
	kh := sftpTestKnownHosts(t, ln.Addr().String(), hostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = deploySFTP(ctx, Options{
		Dir: writeSite(t), Target: "sftp://tester@" + ln.Addr().String() + "/p", Quiet: true,
		Env: func(k string) string { return map[string]string{"SSH_KEY_FILE": keyFile, "SSH_KNOWN_HOSTS": kh}[k] },
	})
	if err == nil {
		t.Error("expected an sftp-open error when the subsystem is rejected")
	}
}

func TestDeploySFTPWalkErrorAfterConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ln.Close() }()
	hostKey := newEd25519Signer(t)
	keyFile, signer := newClientKeyFile(t)
	go serveOneSFTP(t, ln, hostKey, signer.PublicKey())
	kh := sftpTestKnownHosts(t, ln.Addr().String(), hostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Connection/auth succeed; walkFiles then fails on the missing local dir.
	_, err = deploySFTP(ctx, Options{
		Dir: filepath.Join(t.TempDir(), "absent"), Target: "sftp://tester@" + ln.Addr().String() + "/p", Quiet: true,
		Env: func(k string) string { return map[string]string{"SSH_KEY_FILE": keyFile, "SSH_KNOWN_HOSTS": kh}[k] },
	})
	if err == nil {
		t.Error("expected a walk error for a missing local directory")
	}
}

func TestCloudflareUploadTokenBadResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Valid envelope but result is a scalar → uploadToken's Unmarshal into a struct fails.
		_, _ = w.Write([]byte(`{"success":true,"result":"scalar"}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected an upload-token decode error")
	}
}

func TestDeploySFTPCreateError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ln.Close() }()
	hostKey := newEd25519Signer(t)
	keyFile, signer := newClientKeyFile(t)
	go serveOneSFTP(t, ln, hostKey, signer.PublicKey())
	kh := sftpTestKnownHosts(t, ln.Addr().String(), hostKey)

	// The remote base already contains a *directory* named "index.html", so creating
	// the file of the same name fails (MkdirAll succeeds, Create does not).
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "index.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = deploySFTP(ctx, Options{
		Dir: writeSite(t), Target: "sftp://tester@" + ln.Addr().String() + base, Quiet: true,
		Env: func(k string) string { return map[string]string{"SSH_KEY_FILE": keyFile, "SSH_KNOWN_HOSTS": kh}[k] },
	})
	if err == nil {
		t.Error("expected a create error when the target file name is a directory")
	}
}
