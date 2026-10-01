package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxJingleBytes caps a jingle: it is a few seconds of sound, not an episode.
const maxJingleBytes = 8 << 20

// FetchJingle downloads the intro played before every article. Only http and
// https URLs are accepted, the response must be 2xx, and it is capped in size.
// The caller caches the result: a jingle is fetched once, not once per page.
func FetchJingle(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("tts: jingle_url %q is not an http(s) URL", rawURL)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ssg-tts")
	resp, err := (&http.Client{Timeout: timeout}).Do(req) // #nosec G704 -- jingle_url is the site owner's own config
	if err != nil {
		return nil, fmt.Errorf("tts: fetching jingle: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("tts: fetching jingle: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJingleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("tts: fetching jingle: %w", err)
	}
	if len(data) > maxJingleBytes {
		return nil, fmt.Errorf("tts: jingle larger than %d MB", maxJingleBytes>>20)
	}
	return data, nil
}
