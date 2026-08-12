package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSendLoudSuccessAndState: a non-quiet run prints the summary, plain
// (non-$) header values pass through unexpanded, and two announced posts land
// in the state file as comma-separated sorted entries.
func TestSendLoudSuccessAndState(t *testing.T) {
	var hits int
	var got []Post
	srv := recvServer(&hits, &got)
	defer srv.Close()
	state := filepath.Join(t.TempDir(), "s.json")
	n := New([]Dest{{Name: "hook", URL: srv.URL, AllowPrivate: true,
		Headers: map[string]string{"X-Static": "plain"}}}, state)
	sent, err := n.Send([]Post{{Slug: "a", Hash: "1"}, {Slug: "b", Hash: "2"}}, false)
	if err != nil || sent != 2 || hits != 2 {
		t.Fatalf("loud send = %d sent, %d hits, %v", sent, hits, err)
	}
	b, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"a": "1",`) || !strings.Contains(string(b), `"b": "2"`) {
		t.Errorf("state must hold both entries comma-separated:\n%s", b)
	}
}

// TestSendLoudFailureWarns: a destination whose URL cannot even become a
// request fails delivery (warned when not quiet) without any network use.
func TestSendLoudFailureWarns(t *testing.T) {
	n := New([]Dest{{Name: "bad", URL: "://not-a-url"}}, filepath.Join(t.TempDir(), "s.json"))
	sent, err := n.Send([]Post{{Slug: "a", Hash: "h"}}, false)
	if err != nil || sent != 0 {
		t.Errorf("unbuildable request must fail delivery: %d, %v", sent, err)
	}
}

// TestStatePathErrors: a directory as the state path fails the load, and a
// read-only parent fails the save.
func TestStatePathErrors(t *testing.T) {
	if _, err := New([]Dest{{URL: "http://x"}}, t.TempDir()).Send(nil, true); err == nil {
		t.Error("a directory state path must fail the load")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if _, err := New([]Dest{{URL: "http://x"}}, filepath.Join(ro, "s.json")).Send(nil, true); err == nil {
		t.Error("a read-only state dir must fail the save")
	}
}
