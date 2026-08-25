package mcp

// What a media file may be, where it may come from, and who still points at it
// (#214).
//
// Three rules shape this, and each of them is a mistake this project has
// already made once:
//
//   - A file is judged by its bytes, not its name. The extension decides
//     nothing — SEC-013 wrote that down, and #198 was the AVIF pass forgetting
//     it a release later.
//   - A URL is fetched through the same guard external sources use. A server
//     that downloads whatever it is told to is a proxy into the network it runs
//     on, and "fetch this and put it on the site" is precisely the shape that
//     reaches an internal service.
//   - A file nothing references may go; one something references may not. A
//     broken image is a worse outcome than an unused file.

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/images"
)

// maxMediaBytes caps an upload: generous for a photograph, far below what fills
// a disk by accident.
const maxMediaBytes = 20 << 20

// mediaFetchTimeout bounds a download. A tool call that hangs is a session that
// hangs.
const mediaFetchTimeout = 30 * time.Second

// mediaBases is where media may be written: the static directories, which is
// where a site's images live and what the build copies verbatim.
func (s *Server) mediaBases() []string { return s.opts.StaticDirs }

// mediaKind names the format from the file's leading bytes, or says why it is
// not storable.
//
// The allow-list is the same one the image pipeline processes, because storing
// something ssg cannot process means a file the site serves and the build
// cannot resize, convert or check. SVG is deliberately outside it: it is XML
// that can carry script, served from the site's own origin, and "it is an
// image" is exactly the reasoning that makes it dangerous.
func mediaKind(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("the file is empty")
	}
	if len(data) > maxMediaBytes {
		return "", fmt.Errorf("the file is %d bytes and the limit is %d", len(data), maxMediaBytes)
	}
	format, ok := sniffImage(data)
	if !ok || !images.Decodable(format) {
		return "", fmt.Errorf("this is not a JPEG, PNG, GIF or WebP — the file's own bytes decide, " +
			"whatever its name says, and nothing else is stored")
	}
	return format, nil
}

// sniffImage reads a format from the magic bytes.
func sniffImage(data []byte) (string, bool) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpeg", true
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "png", true
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "gif", true
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp", true
	}
	return "", false
}

// mediaBytes resolves what an upload is carrying, and says where it came from
// so the reply can tell the operator which of the two paths ran.
func (s *Server) mediaBytes(args map[string]any) (data []byte, origin string, err error) {
	encoded, _ := rawArg(args, "content_base64")
	link, _ := strArg(args, "url")
	encoded, link = strings.TrimSpace(encoded), strings.TrimSpace(link)

	switch {
	case encoded != "" && link != "":
		return nil, "", fmt.Errorf("give either `content_base64` or `url`, not both")
	case encoded != "":
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("`content_base64` is not valid base64: %w", err)
		}
		return data, "base64", nil
	case link != "":
		data, err := s.fetchMedia(link)
		return data, link, err
	}
	return nil, "", fmt.Errorf("`content_base64` or `url` is required")
}

// fetchMedia downloads a URL through the transport that refuses private
// addresses, resolving the host itself so a DNS answer changing between check
// and connection cannot reach one either.
func (s *Server) fetchMedia(raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("`url` is not a URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("`url` must be http or https, got %q", u.Scheme)
	}

	client := &http.Client{
		Timeout:   mediaFetchTimeout,
		Transport: externalsource.SecureTransport(s.opts.MediaAllowPrivate),
	}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", safeURL(u), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: the server answered %d", safeURL(u), resp.StatusCode)
	}
	// One byte past the cap, so a file that is exactly too large is refused by
	// mediaKind rather than silently truncated to the limit.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", safeURL(u), err)
	}
	return data, nil
}

// mediaReferences returns the project files that mention this path.
//
// By the name as written and by the bare filename: a page may link
// `/images/logo.png` where the file is `static/images/logo.png`, and matching
// only the full relative path would report "nothing uses it" about a file three
// pages display.
func (s *Server) mediaReferences(rel string) []string {
	needles := referenceForms(rel)
	var out []string
	for _, base := range append(append([]string{}, s.opts.ContentDirs...), s.opts.TemplateDirs...) {
		files, err := listFiles(s.opts.Root, []string{base})
		if err != nil {
			continue
		}
		for _, candidate := range files {
			body, readErr := readFile(joinProject(s.opts.Root, candidate))
			if readErr != nil {
				continue
			}
			if mentionsAny(body, needles) {
				out = append(out, candidate)
			}
		}
	}
	return out
}

// referenceForms lists the spellings a document might use for a media path.
func referenceForms(rel string) []string {
	rel = strings.TrimPrefix(rel, "./")
	forms := []string{rel, "/" + rel}
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		// The path as the site serves it: static/images/x.png is /images/x.png,
		// because the static directory itself is not part of the URL.
		if cut := strings.Index(rel, "/"); cut >= 0 {
			served := rel[cut:]
			forms = append(forms, served, strings.TrimPrefix(served, "/"))
		}
		forms = append(forms, rel[i+1:])
	}
	return forms
}

// mentionsAny reports whether body contains any of the needles.
func mentionsAny(body string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(body, n) {
			return true
		}
	}
	return false
}

// safeURL renders a URL for a message: scheme, host and path, and nothing else.
//
// url.Redacted() hides the password and keeps the query string, which is where
// a token usually is — a signed media link, an API key on a CDN. An error
// message is the least controlled place a secret can end up: it reaches the
// model, the transcript, and whatever logs the operator keeps.
func safeURL(u *url.URL) string {
	trimmed := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	if u.RawQuery != "" {
		return trimmed.String() + "?…"
	}
	return trimmed.String()
}
