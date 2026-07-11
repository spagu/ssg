package deploy

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- Netlify content-addresses files by SHA-1 (digest key, not a security use)
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// netlifyAPIBase is a var (not const) so tests can point it at a mock server.
var netlifyAPIBase = "https://api.netlify.com/api/v1"

// deployNetlify publishes o.Dir to a Netlify site via the digest-based deploy API:
// declare every file's SHA-1, then upload only the digests Netlify reports missing.
func deployNetlify(ctx context.Context, o Options) (string, error) {
	site := o.Project
	if site == "" {
		site = o.env("NETLIFY_SITE_ID")
	}
	token := o.env("NETLIFY_AUTH_TOKEN")
	if site == "" || token == "" {
		return "", fmt.Errorf("netlify needs --deploy-project (site ID) and NETLIFY_AUTH_TOKEN")
	}

	files, err := walkFiles(o.Dir)
	if err != nil {
		return "", fmt.Errorf("scanning output: %w", err)
	}
	digests := make(map[string]string, len(files)) // "/path" → sha1
	byPath := make(map[string][]byte, len(files))
	for _, f := range files {
		// #nosec G401 -- Netlify content-addresses files by SHA-1 (digest key, not a security use)
		sum := sha1.Sum(f.Data) // NOSONAR S4790: SHA-1 is Netlify's content-address key per its API, not a security use
		path := "/" + f.Rel
		digests[path] = hex.EncodeToString(sum[:])
		byPath[path] = f.Data
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	deployID, required, deployURL, err := netlifyCreateDeploy(ctx, client, netlifyAPIBase, site, token, digests)
	if err != nil {
		return "", fmt.Errorf("creating deploy: %w", err)
	}
	o.logf("☁️  Uploading %d/%d files to Netlify…", len(required), len(files))
	for path, data := range byPath {
		if !required[digests[path]] {
			continue
		}
		if err := netlifyUpload(ctx, client, netlifyAPIBase, deployID, token, path, data); err != nil {
			return "", fmt.Errorf("uploading %s: %w", path, err)
		}
	}
	return deployURL, nil
}

// netlifyCreateDeploy declares the file manifest and returns the deploy id, the set
// of SHA-1 digests still to upload, and the deploy's public URL.
func netlifyCreateDeploy(ctx context.Context, client *http.Client, base, site, token string, digests map[string]string) (string, map[string]bool, string, error) {
	body, _ := json.Marshal(map[string]any{"files": digests})
	url := fmt.Sprintf("%s/sites/%s/deploys", base, site)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return "", nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	var out struct {
		ID       string   `json:"id"`
		Required []string `json:"required"`
		SSLURL   string   `json:"ssl_url"`
		URL      string   `json:"url"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", nil, "", err
	}
	required := make(map[string]bool, len(out.Required))
	for _, h := range out.Required {
		required[h] = true
	}
	url2 := out.SSLURL
	if url2 == "" {
		url2 = out.URL
	}
	return out.ID, required, url2, nil
}

// netlifyUpload PUTs one file's bytes into the deploy.
func netlifyUpload(ctx context.Context, client *http.Client, base, deployID, token, path string, data []byte) error {
	url := fmt.Sprintf("%s/deploys/%s/files%s", base, deployID, path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}
