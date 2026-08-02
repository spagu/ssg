// Package notify announces newly published (or changed) posts to user-defined
// webhook destinations. A committed state file dedupes, so a post is sent once —
// again only when its content changes. It never sends unless explicitly enabled
// (--notify), so dev builds stay quiet (#1.8.16).
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/externalsource"
)

// Dest is one webhook destination.
type Dest struct {
	Name, URL, Method string
	Headers           map[string]string
	AllowPrivate      bool
}

// Post is the payload announced for one post; Hash is the dedup key (not sent).
type Post struct {
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	URL     string   `json:"url"`
	Excerpt string   `json:"excerpt,omitempty"`
	Date    string   `json:"date,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Hash    string   `json:"-"`
}

// Notifier sends posts to destinations and tracks what has already been sent.
type Notifier struct {
	dests     []Dest
	statePath string
	timeout   time.Duration
}

// New builds a notifier. statePath defaults to ".ssg-notifications.json".
func New(dests []Dest, statePath string) *Notifier {
	if statePath == "" {
		statePath = ".ssg-notifications.json"
	}
	return &Notifier{dests: dests, statePath: statePath, timeout: 15 * time.Second}
}

// Enabled reports whether any destination is configured.
func (n *Notifier) Enabled() bool { return n != nil && len(n.dests) > 0 }

// Send announces each post whose hash differs from the recorded state (new or
// changed), updating the state only for posts delivered to every destination.
// Returns the number of posts sent. Delivery errors are reported but do not abort
// the others; a post that fails any destination is retried on the next run.
func (n *Notifier) Send(posts []Post, quiet bool) (int, error) {
	state, err := loadState(n.statePath)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, p := range posts {
		if state[p.Slug] == p.Hash {
			continue // already announced at this content hash
		}
		if n.deliver(p, quiet) {
			state[p.Slug] = p.Hash
			sent++
		}
	}
	if err := saveState(n.statePath, state); err != nil {
		return sent, err
	}
	if !quiet && sent > 0 {
		fmt.Printf("   📣 Notified %d post(s) to %d destination(s)\n", sent, len(n.dests))
	}
	return sent, nil
}

// deliver POSTs one post to every destination; reports whether all succeeded.
func (n *Notifier) deliver(p Post, quiet bool) bool {
	body, _ := json.Marshal(p)
	ok := true
	for _, d := range n.dests {
		if err := n.post(d, body); err != nil {
			ok = false
			if !quiet {
				fmt.Printf("   ⚠️  notify %s (%s): %v\n", d.Name, p.Slug, err)
			}
		}
	}
	return ok
}

// post sends the payload to one destination over an SSRF-hardened transport.
func (n *Notifier) post(d Dest, body []byte) error {
	method := d.Method
	if method == "" {
		method = http.MethodPost
	}
	// #nosec G704 -- d.URL is author config (not request-controlled); the transport
	// refuses private/loopback ranges at dial time unless allow_private is set.
	req, err := http.NewRequest(method, d.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range d.Headers {
		req.Header.Set(k, expandEnv(v))
	}
	client := &http.Client{Timeout: n.timeout, Transport: externalsource.SecureTransport(d.AllowPrivate)}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("destination returned %d", resp.StatusCode)
	}
	return nil
}

func loadState(path string) (map[string]string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- state path is author config
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	state := map[string]string{}
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, fmt.Errorf("notify state %s: %w", path, err)
	}
	return state, nil
}

func saveState(path string, state map[string]string) error {
	// Sorted keys keep the committed state file diff-friendly.
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		vb, _ := json.Marshal(state[k])
		fmt.Fprintf(&b, "  %s: %s", kb, vb)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	// #nosec G306 -- the notification state is a non-sensitive, committable file
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func expandEnv(v string) string {
	if strings.HasPrefix(v, "$") {
		return os.Getenv(strings.TrimPrefix(v, "$"))
	}
	return v
}
