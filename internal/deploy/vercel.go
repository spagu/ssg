package deploy

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- Vercel's upload API keys files by SHA-1 digest; not used for security
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// vercelAPIBase is a var (not const) so tests can point it at a mock server.
var vercelAPIBase = "https://api.vercel.com"

// vercelFile is one entry in the create-deployment files array.
type vercelFile struct {
	File string `json:"file"`
	SHA  string `json:"sha"`
	Size int    `json:"size"`
}

// deployVercel publishes o.Dir by uploading each file (keyed by SHA-1) and then
// creating a deployment that references them.
func deployVercel(ctx context.Context, o Options) (string, error) {
	project := o.Project
	if project == "" {
		project = o.env("VERCEL_PROJECT_ID")
	}
	token := o.env("VERCEL_TOKEN")
	if project == "" || token == "" {
		return "", fmt.Errorf("vercel needs --deploy-project and VERCEL_TOKEN")
	}
	teamID := o.env("VERCEL_ORG_ID")

	files, err := walkFiles(o.Dir)
	if err != nil {
		return "", fmt.Errorf("scanning output: %w", err)
	}
	client := &http.Client{Timeout: 5 * time.Minute}

	manifest := make([]vercelFile, 0, len(files))
	seen := map[string]bool{}
	for _, f := range files {
		sum := sha1.Sum(f.Data) //nolint:gosec // NOSONAR S4790: Vercel content-addresses files by SHA-1; digest key, not a security use
		sha := hex.EncodeToString(sum[:])
		manifest = append(manifest, vercelFile{File: f.Rel, SHA: sha, Size: len(f.Data)})
		if seen[sha] {
			continue
		}
		seen[sha] = true
		if err := vercelUpload(ctx, client, token, teamID, sha, f.Data); err != nil {
			return "", fmt.Errorf("uploading %s: %w", f.Rel, err)
		}
	}
	o.logf("☁️  Uploaded %d files to Vercel; creating deployment…", len(seen))
	return vercelCreateDeployment(ctx, client, token, teamID, project, manifest)
}

// vercelUpload POSTs one file's bytes, keyed by its SHA-1 digest.
func vercelUpload(ctx context.Context, client *http.Client, token, teamID, sha string, data []byte) error {
	url := vercelAPIBase + "/v2/files"
	if teamID != "" {
		url += "?teamId=" + teamID
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-vercel-digest", sha)
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

// vercelCreateDeployment creates the deployment referencing the uploaded files and
// returns its public https URL.
func vercelCreateDeployment(ctx context.Context, client *http.Client, token, teamID, project string, files []vercelFile) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"name":            project,
		"project":         project,
		"target":          "production",
		"files":           files,
		"projectSettings": map[string]any{"framework": nil},
	})
	url := vercelAPIBase + "/v13/deployments"
	if teamID != "" {
		url += "?teamId=" + teamID
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", nil
	}
	return "https://" + out.URL, nil
}
