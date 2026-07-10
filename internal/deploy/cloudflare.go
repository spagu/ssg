package deploy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// defaultAPIBase is the Cloudflare v4 API root. It is a var (not const) so tests can
// point it at a mock server; the client copies it into its baseURL field.
var defaultAPIBase = "https://api.cloudflare.com/client/v4"

// uploadBatchFiles caps how many assets go in one /pages/assets/upload request.
const uploadBatchFiles = 200

// CloudflareConfig carries the credentials and target for a Pages Direct Upload.
// Credentials come from the environment (CLOUDFLARE_API_TOKEN / CLOUDFLARE_ACCOUNT_ID)
// rather than the on-disk config, so secrets never live in a committed file.
type CloudflareConfig struct {
	AccountID string
	APIToken  string
	Project   string
	Branch    string // optional; defaults to the project's production branch
	Quiet     bool
}

// CloudflarePages deploys a directory to Cloudflare Pages via the Direct Upload API.
type CloudflarePages struct {
	cfg     CloudflareConfig
	client  *http.Client
	baseURL string
}

// NewCloudflarePages builds a deployer with a sane HTTP timeout.
func NewCloudflarePages(cfg CloudflareConfig) *CloudflarePages {
	return &CloudflarePages{
		cfg:     cfg,
		client:  &http.Client{Timeout: 5 * time.Minute},
		baseURL: defaultAPIBase,
	}
}

// deployCloudflare adapts the generic deploy Options to a Cloudflare Pages upload,
// reading credentials from the environment.
func deployCloudflare(ctx context.Context, o Options) (string, error) {
	return NewCloudflarePages(CloudflareConfig{
		AccountID: o.env("CLOUDFLARE_ACCOUNT_ID"),
		APIToken:  o.env("CLOUDFLARE_API_TOKEN"),
		Project:   o.Project,
		Branch:    o.Branch,
		Quiet:     o.Quiet,
	}).Deploy(ctx, o.Dir)
}

// Validate reports whether the required credentials/target are present.
func (c *CloudflarePages) Validate() error {
	switch {
	case c.cfg.APIToken == "":
		return fmt.Errorf("missing CLOUDFLARE_API_TOKEN")
	case c.cfg.AccountID == "":
		return fmt.Errorf("missing CLOUDFLARE_ACCOUNT_ID")
	case c.cfg.Project == "":
		return fmt.Errorf("missing Cloudflare Pages project name (--cf-project)")
	}
	return nil
}

// envelope is the standard Cloudflare API response wrapper.
type envelope struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  json.RawMessage   `json:"result"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e envelope) err() error {
	if e.Success {
		return nil
	}
	if len(e.Errors) > 0 {
		return fmt.Errorf("cloudflare API error %d: %s", e.Errors[0].Code, e.Errors[0].Message)
	}
	return fmt.Errorf("cloudflare API request was not successful")
}

// do sends a request with the given bearer token and decodes the envelope, returning
// its raw result on success.
func (c *CloudflarePages) do(ctx context.Context, method, url, bearer, contentType string, body io.Reader) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decoding response (HTTP %d): %w", resp.StatusCode, err)
	}
	if err := env.err(); err != nil {
		return nil, err
	}
	return env.Result, nil
}

// Deploy uploads dir to Cloudflare Pages and returns the deployment URL.
func (c *CloudflarePages) Deploy(ctx context.Context, dir string) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	files, err := collectSiteFiles(dir)
	if err != nil {
		return "", fmt.Errorf("scanning output: %w", err)
	}
	if len(files.assets) == 0 {
		return "", fmt.Errorf("no files to deploy in %q", dir)
	}

	jwt, err := c.uploadToken(ctx)
	if err != nil {
		return "", fmt.Errorf("getting upload token: %w", err)
	}

	uniq := files.uniqueByHash()
	missing, err := c.checkMissing(ctx, jwt, uniq)
	if err != nil {
		return "", fmt.Errorf("checking existing assets: %w", err)
	}
	c.logf("☁️  Uploading %d/%d assets to Cloudflare Pages…", len(missing), len(uniq))
	if err := c.uploadAssets(ctx, jwt, missing); err != nil {
		return "", fmt.Errorf("uploading assets: %w", err)
	}

	url, err := c.createDeployment(ctx, files)
	if err != nil {
		return "", fmt.Errorf("creating deployment: %w", err)
	}
	c.logf("✅ Deployed to %s", url)
	return url, nil
}

// uploadToken fetches a short-lived JWT scoped to this project's asset endpoints.
func (c *CloudflarePages) uploadToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/upload-token", c.baseURL, c.cfg.AccountID, c.cfg.Project)
	res, err := c.do(ctx, http.MethodGet, url, c.cfg.APIToken, "", nil)
	if err != nil {
		return "", err
	}
	var out struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", err
	}
	if out.JWT == "" {
		return "", fmt.Errorf("empty upload token in response")
	}
	return out.JWT, nil
}

// checkMissing asks Cloudflare which of the asset hashes still need uploading and
// returns the subset of assets to send.
func (c *CloudflarePages) checkMissing(ctx context.Context, jwt string, assets []asset) ([]asset, error) {
	hashes := make([]string, len(assets))
	for i, a := range assets {
		hashes[i] = a.Hash
	}
	body, _ := json.Marshal(map[string][]string{"hashes": hashes})
	res, err := c.do(ctx, http.MethodPost, c.baseURL+"/pages/assets/check-missing", jwt, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var missingHashes []string
	if err := json.Unmarshal(res, &missingHashes); err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(missingHashes))
	for _, h := range missingHashes {
		want[h] = true
	}
	var missing []asset
	for _, a := range assets {
		if want[a.Hash] {
			missing = append(missing, a)
		}
	}
	return missing, nil
}

// uploadPayload is one entry in the /pages/assets/upload request array.
type uploadPayload struct {
	Key      string            `json:"key"`
	Value    string            `json:"value"`
	Metadata map[string]string `json:"metadata"`
	Base64   bool              `json:"base64"`
}

// uploadAssets uploads the missing assets in bounded batches.
func (c *CloudflarePages) uploadAssets(ctx context.Context, jwt string, assets []asset) error {
	for start := 0; start < len(assets); start += uploadBatchFiles {
		end := start + uploadBatchFiles
		if end > len(assets) {
			end = len(assets)
		}
		batch := make([]uploadPayload, 0, end-start)
		for _, a := range assets[start:end] {
			batch = append(batch, uploadPayload{
				Key:      a.Hash,
				Value:    base64.StdEncoding.EncodeToString(a.Content),
				Metadata: map[string]string{"contentType": a.ContentType},
				Base64:   true,
			})
		}
		body, _ := json.Marshal(batch)
		if _, err := c.do(ctx, http.MethodPost, c.baseURL+"/pages/assets/upload", jwt, "application/json", bytes.NewReader(body)); err != nil {
			return err
		}
	}
	return nil
}

// createDeployment posts the asset manifest and any control files, returning the
// deployment's public URL.
func (c *CloudflarePages) createDeployment(ctx context.Context, files *siteFiles) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	manifestJSON, _ := json.Marshal(files.manifest())
	if err := mw.WriteField("manifest", string(manifestJSON)); err != nil {
		return "", err
	}
	if c.cfg.Branch != "" {
		if err := mw.WriteField("branch", c.cfg.Branch); err != nil {
			return "", err
		}
	}
	for name, content := range files.special {
		if err := mw.WriteField(name, content); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments", c.baseURL, c.cfg.AccountID, c.cfg.Project)
	res, err := c.do(ctx, http.MethodPost, url, c.cfg.APIToken, mw.FormDataContentType(), &buf)
	if err != nil {
		return "", err
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *CloudflarePages) logf(format string, args ...interface{}) {
	if !c.cfg.Quiet {
		fmt.Printf(format+"\n", args...)
	}
}
