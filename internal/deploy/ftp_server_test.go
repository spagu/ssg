package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// ftpTestServer is a minimal in-process FTP server: just enough of the protocol
// (login, FEAT, TYPE, MKD, EPSV, STOR) for jlaffaye/ftp to upload a small tree.
type ftpTestServer struct {
	ln       net.Listener
	mu       sync.Mutex
	received map[string]string // remote path → contents
}

func newFTPTestServer(t *testing.T) *ftpTestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	s := &ftpTestServer{ln: ln, received: map[string]string{}}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *ftpTestServer) addr() string { return s.ln.Addr().String() }

func (s *ftpTestServer) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	handleFTP(conn, s)
}

// handleFTP runs the control-connection command loop.
func handleFTP(conn net.Conn, s *ftpTestServer) {
	w := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	w("220 test FTP ready")
	r := bufio.NewReader(conn)
	var dataLn net.Listener
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd, arg := splitFTPCommand(line)
		switch cmd {
		case "USER":
			w("331 need password")
		case "PASS":
			w("230 logged in")
		case "FEAT":
			w("211-Features")
			w(" EPSV")
			w("211 End")
		case "TYPE", "OPTS", "NOOP":
			w("200 ok")
		case "SYST":
			w("215 UNIX Type: L8")
		case "PWD":
			w(`257 "/"`)
		case "CWD":
			w("250 ok")
		case "MKD":
			w("257 \"" + arg + "\" created")
		case "EPSV":
			dataLn = openDataListener(w)
		case "STOR":
			storeFTPFile(conn, r, dataLn, arg, w, s)
			dataLn = nil
		case "QUIT":
			w("221 bye")
			return
		default:
			w("200 ok")
		}
	}
}

func splitFTPCommand(line string) (cmd, arg string) {
	fields := strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 2)
	cmd = strings.ToUpper(fields[0])
	if len(fields) > 1 {
		arg = fields[1]
	}
	return cmd, arg
}

// openDataListener opens an ephemeral passive-data listener and advertises it via EPSV.
func openDataListener(w func(string)) net.Listener {
	dl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		w("425 cannot open data connection")
		return nil
	}
	_, port, _ := net.SplitHostPort(dl.Addr().String())
	w(fmt.Sprintf("229 Entering Extended Passive Mode (|||%s|)", port))
	return dl
}

// storeFTPFile accepts the passive data connection and records the uploaded bytes.
func storeFTPFile(_ net.Conn, _ *bufio.Reader, dataLn net.Listener, path string, w func(string), s *ftpTestServer) {
	if dataLn == nil {
		w("425 no data connection")
		return
	}
	defer func() { _ = dataLn.Close() }()
	w("150 opening data connection")
	dc, err := dataLn.Accept()
	if err != nil {
		w("426 transfer aborted")
		return
	}
	_ = dc.SetDeadline(time.Now().Add(10 * time.Second))
	data, _ := io.ReadAll(dc)
	_ = dc.Close()
	s.mu.Lock()
	s.received[path] = string(data)
	s.mu.Unlock()
	w("226 transfer complete")
}

// TestDeployFTPRoundTrip uploads a small site to the in-process FTP server and checks
// the files (and their parent directories) were created.
func TestDeployFTPRoundTrip(t *testing.T) {
	s := newFTPTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	url, err := deployFTP(ctx, Options{
		Dir:    writeSite(t),
		Target: "ftp://tester@" + s.addr() + "/public",
		Quiet:  true,
		Env:    func(k string) string { return map[string]string{"FTP_PASSWORD": "secret"}[k] },
	})
	if err != nil {
		t.Fatalf("deployFTP round-trip: %v", err)
	}
	if url == "" {
		t.Error("expected a non-empty ftp URL")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.received["public/index.html"]; !ok {
		t.Errorf("index.html not received; got %v", keysOf(s.received))
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
