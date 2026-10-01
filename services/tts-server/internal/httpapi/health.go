package httpapi

import "net/http"

// healthz is liveness: the process is up and serving HTTP.
func (a *api) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz is readiness: engine binaries exist and at least one voice loaded.
func (a *api) readyz(w http.ResponseWriter, _ *http.Request) {
	if err := a.Ready(); err != nil {
		writeError(w, http.StatusServiceUnavailable, codeNotReady, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "voices": a.Registry.Catalog().Len()})
}
