package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

//go:embed docs.html
var docsPage []byte

//go:embed swagger-init.js
var docsInit []byte

// docsCSP lets the Swagger UI page load only the pinned swagger-ui-dist
// assets from jsDelivr and talk only to this server.
const docsCSP = "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net; " +
	"style-src https://cdn.jsdelivr.net 'unsafe-inline'; img-src 'self' data: https://cdn.jsdelivr.net; " +
	"font-src https://cdn.jsdelivr.net data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'"

// serveSpec returns the OpenAPI 3.1 document.
func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(openAPISpec)
}

// serveDocs returns the Swagger UI page.
func serveDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", docsCSP)
	_, _ = w.Write(docsPage)
}

// serveDocsInit returns the script that boots Swagger UI (kept out of the
// page so the CSP needs no 'unsafe-inline' for scripts).
func serveDocsInit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(docsInit)
}
