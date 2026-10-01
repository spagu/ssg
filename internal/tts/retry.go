package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxAudioBytes caps one response: an MP3 of a long article is a few MB, and a
// misbehaving endpoint must not be able to fill the disk or the memory.
const maxAudioBytes = 64 << 20

// maxRetryAfter caps how long a Retry-After header may make a build wait.
const maxRetryAfter = 30 * time.Second

// statusError is a non-2xx answer: the status, and the start of the body,
// which is where every provider puts its explanation.
type statusError struct {
	code int
	body string
	wait time.Duration // from Retry-After, 0 when absent
}

func (e *statusError) Error() string {
	return fmt.Sprintf("tts: HTTP %d: %s", e.code, e.body)
}

// retryable reports whether an attempt is worth repeating: a network error,
// a timeout, 429 or a 5xx. A 4xx other than 429 is the request's fault —
// a bad key, an unknown voice — and asking again changes nothing.
func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var se *statusError
	if errors.As(err, &se) {
		return se.code == http.StatusTooManyRequests || se.code >= 500
	}
	return true
}

// call sends one chunk, retrying with exponential backoff (1s, 2s, 4s…) or
// what Retry-After asks for, whichever is longer, capped at 30s.
func (c *Client) call(ctx context.Context, text, lang string) ([]byte, error) {
	var err error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			wait := time.Second << (attempt - 1)
			var se *statusError
			if errors.As(err, &se) && se.wait > wait {
				wait = se.wait
			}
			if sleepErr := c.sleep(ctx, wait); sleepErr != nil {
				return nil, sleepErr
			}
		}
		var audio []byte
		audio, err = c.once(ctx, text, lang)
		if err == nil {
			return audio, nil
		}
		if !retryable(err) {
			return nil, err
		}
	}
	return nil, err
}

// once performs a single request.
func (c *Client) once(ctx context.Context, text, lang string) ([]byte, error) {
	req, err := c.provider.newRequest(ctx, c.cfg, text, lang)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req) // #nosec G704 -- the endpoint is the site owner's own tts.api_url
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &statusError{code: resp.StatusCode, body: snippet(body), wait: retryAfter(resp.Header.Get("Retry-After"))}
	}
	if len(body) > maxAudioBytes {
		return nil, fmt.Errorf("tts: response larger than %d MB", maxAudioBytes>>20)
	}
	return c.provider.decode(body)
}

// retryAfter parses a Retry-After given in seconds, capped; dates and
// garbage count as absent.
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}

// snippet is the start of an error body, on one line.
func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
