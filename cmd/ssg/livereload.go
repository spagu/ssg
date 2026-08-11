package main

// Live reload for `--http --watch` (GO-090): the built-in server exposes a tiny
// long-lived endpoint the page holds open and waits on. After a successful
// rebuild the server pushes a "reload" event and the browser refreshes; when a
// rebuild FAILS it pushes the error, and the page shows a dismissable overlay
// bar with the message instead of silently serving the stale page. No external
// process — it is part of the dev server, active only with --auto-reload
// (on by default in --http --watch).

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const liveReloadPath = "/__livereload"

// liveReloadScript subscribes the tab to rebuild events: "reload" refreshes the
// page; "builderror" shows a fixed error bar (white on dark red, role=alert for
// assistive tech). A successful reload clears the bar by loading a fresh page.
const liveReloadScript = `<script>(function(){try{var s=new EventSource("` + liveReloadPath + `");` +
	`s.addEventListener("reload",function(){location.reload()});` +
	`s.addEventListener("builderror",function(e){var i="__ssg_builderror",el=document.getElementById(i);` +
	`if(!el){el=document.createElement("div");el.id=i;el.setAttribute("role","alert");` +
	`el.style.cssText="position:fixed;left:0;right:0;bottom:0;z-index:2147483647;background:#7f1d1d;color:#fff;` +
	`font:14px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;padding:12px 16px;white-space:pre-wrap;` +
	`max-height:50vh;overflow:auto;box-shadow:0 -2px 12px rgba(0,0,0,.4)";document.body.appendChild(el)}` +
	`el.textContent="⚠ Build failed\n"+e.data})}catch(e){}})();</script>`

// liveReloadHub fans a rebuild payload out to every connected browser tab. The
// payload is a ready-to-write SSE event.
type liveReloadHub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

// reloadHub is non-nil only while --auto-reload is active.
var reloadHub *liveReloadHub

func newLiveReloadHub() *liveReloadHub {
	return &liveReloadHub{subs: make(map[chan string]struct{})}
}

func (h *liveReloadHub) subscribe() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *liveReloadHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *liveReloadHub) broadcast(payload string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- payload:
		default: // slow client; drop rather than block the build
		}
	}
}

// sseEvent formats a named SSE event, splitting a multi-line message into the
// data: lines the protocol requires.
func sseEvent(event, data string) string {
	var b strings.Builder
	b.WriteString("event: " + event + "\n")
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		b.WriteString("data: " + line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// notifyReload signals a successful rebuild (no-op unless the hub is running).
func notifyReload() {
	if reloadHub != nil {
		reloadHub.broadcast(sseEvent("reload", "1"))
	}
}

// notifyBuildError pushes a failed build's message to the overlay (no-op unless
// the hub is running).
func notifyBuildError(msg string) {
	if reloadHub != nil {
		reloadHub.broadcast(sseEvent("builderror", msg))
	}
}

// serveSSE holds the connection open and streams each rebuild payload.
func (h *liveReloadHub) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.subscribe()
	defer h.unsubscribe(ch)
	_, _ = w.Write([]byte(": connected\n\n")) // open the stream
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write([]byte(payload))
			flusher.Flush()
		}
	}
}

// liveReloadMiddleware serves the SSE endpoint and injects the client script
// into HTML responses. A no-op wrapper unless --auto-reload created the hub.
func liveReloadMiddleware(next http.Handler) http.Handler {
	if reloadHub == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == liveReloadPath {
			reloadHub.serveSSE(w, r)
			return
		}
		// Always serve fresh HTML so the injected script is never cached away.
		r.Header.Del("If-None-Match")
		r.Header.Del("If-Modified-Since")
		rec := &htmlInjectWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		rec.finish()
	})
}

// htmlInjectWriter buffers a response so the live-reload script can be inserted
// into HTML before the body is written. Non-HTML (and gzip-encoded) responses
// pass through untouched.
type htmlInjectWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (w *htmlInjectWriter) WriteHeader(code int) { w.status = code }

func (w *htmlInjectWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }

func (w *htmlInjectWriter) finish() {
	body := w.buf.Bytes()
	ct := w.Header().Get("Content-Type")
	encoded := w.Header().Get("Content-Encoding") != ""
	if strings.HasPrefix(ct, "text/html") && !encoded {
		s := string(body)
		if i := strings.LastIndex(s, "</body>"); i >= 0 {
			s = s[:i] + liveReloadScript + s[i:]
		} else {
			s += liveReloadScript
		}
		body = []byte(s)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(body)
}
