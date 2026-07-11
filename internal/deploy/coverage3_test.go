package deploy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/knownhosts"
)

// ── Cloudflare remaining reachable branches ─────────────────────────────────

func TestCloudflareErrEmptyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"errors":[]}`)) // not successful, no error detail
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected the generic not-successful error")
	}
}

func TestCloudflareDeployMissingDir(t *testing.T) {
	c := newCF(t, "http://example.com")
	if _, err := c.Deploy(context.Background(), filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("expected a scanning error for a missing directory")
	}
}

func TestCloudflareUploadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/a/pages/projects/p/upload-token":
			_, _ = w.Write([]byte(`{"success":true,"result":{"jwt":"j"}}`))
		case "/pages/assets/check-missing":
			// Everything is "missing" → forces an upload call, which then fails.
			var body struct {
				Hashes []string `json:"hashes"`
			}
			_ = decodeJSON(r, &body)
			_ = writeResult(w, body.Hashes)
		default: // /pages/assets/upload
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9,"message":"quota"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected an asset-upload error")
	}
}

// ── Netlify: all files already uploaded (skip branch) ───────────────────────

func TestNetlifyNothingRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// required is empty → every file hits the `continue` and nothing is PUT.
		_, _ = w.Write([]byte(`{"id":"d","required":[],"ssl_url":"https://x"}`))
	}))
	t.Cleanup(srv.Close)
	old := netlifyAPIBase
	netlifyAPIBase = srv.URL
	t.Cleanup(func() { netlifyAPIBase = old })
	url, err := deployNetlify(context.Background(), Options{
		Dir: writeSite(t), Project: "s", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "t"}[k] },
	})
	if err != nil || url != "https://x" {
		t.Errorf("deployNetlify nothing-required = %q, %v", url, err)
	}
}

// ── Vercel: duplicate files exercise the dedupe skip ────────────────────────

func TestVercelDuplicateFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("same"), 0o644) // identical → deduped
	uploads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/files" {
			uploads++
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
		Dir: dir, Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t"}[k] },
	}); err != nil {
		t.Fatalf("deployVercel: %v", err)
	}
	if uploads != 1 {
		t.Errorf("identical files uploaded %d times, want 1 (deduped)", uploads)
	}
}

// ── FTP: STOR rejected, and walk error after a successful login ─────────────

func ftpServerRejectStor(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		s := &ftpTestServer{received: map[string]string{}}
		handleFTPRejectStor(conn, s)
	}()
	return ln.Addr().String()
}

// handleFTPRejectStor logs in normally but answers STOR with 550.
func handleFTPRejectStor(conn net.Conn, s *ftpTestServer) {
	w := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	w("220 ready")
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		cmd, _ := splitFTPCommand(string(buf[:n]))
		switch cmd {
		case "USER":
			w("331 need password")
		case "PASS":
			w("230 ok")
		case "FEAT":
			w("211-Features")
			w(" EPSV")
			w("211 End")
		case "EPSV":
			_ = openDataListener(w)
		case "STOR":
			w("550 permission denied")
		case "QUIT":
			w("221 bye")
			return
		default:
			w("200 ok")
		}
	}
}

func TestDeployFTPStorRejected(t *testing.T) {
	addr := ftpServerRejectStor(t)
	_, err := deployFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "ftp://u@" + addr + "/pub", Quiet: true,
		Env: func(k string) string { return map[string]string{"FTP_PASSWORD": "p"}[k] },
	})
	if err == nil {
		t.Error("expected an error when STOR is rejected")
	}
}

func TestDeployFTPWalkError(t *testing.T) {
	s := newFTPTestServer(t)
	// Login succeeds, then walkFiles on a missing directory fails.
	_, err := deployFTP(context.Background(), Options{
		Dir: filepath.Join(t.TempDir(), "absent"), Target: "ftp://u@" + s.addr() + "/pub", Quiet: true,
		Env: func(k string) string { return map[string]string{"FTP_PASSWORD": "p"}[k] },
	})
	if err == nil {
		t.Error("expected a walk error for a missing output directory")
	}
}

// ── SFTP: MkdirAll fails when the remote base sits under a regular file ──────

func TestDeploySFTPWriteError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ln.Close() }()
	hostKey := newEd25519Signer(t)
	keyFile, clientSigner := newClientKeyFile(t)
	go serveOneSFTP(t, ln, hostKey, clientSigner.PublicKey())

	khLine := knownhosts.Line([]string{ln.Addr().String()}, hostKey.PublicKey())
	khFile := filepath.Join(t.TempDir(), "known_hosts")
	_ = os.WriteFile(khFile, []byte(khLine+"\n"), 0o600)

	// A regular file used as if it were a directory → MkdirAll under it fails.
	blocker := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(blocker, []byte("x"), 0o644)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = deploySFTP(ctx, Options{
		Dir:    writeSite(t),
		Target: "sftp://tester@" + ln.Addr().String() + blocker + "/sub",
		Quiet:  true,
		Env:    func(k string) string { return map[string]string{"SSH_KEY_FILE": keyFile, "SSH_KNOWN_HOSTS": khFile}[k] },
	})
	if err == nil {
		t.Error("expected a write error uploading beneath a regular file")
	}
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func writeResult(w http.ResponseWriter, result any) error {
	return json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
}
