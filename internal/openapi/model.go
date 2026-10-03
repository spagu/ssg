// Package openapi reads an OpenAPI 3.0 or 3.1 document (YAML or JSON) into a
// small, render-ready model for documentation pages.
//
// It is a reader for documentation, not a validator: it never panics on bad
// input, keeps document order wherever the author chose one, resolves local
// $refs for parameters, request bodies, responses, headers, examples and
// security schemes, and reports anything it had to skip or found suspicious as
// a Diagnostic carrying a JSON-pointer-like path. Schema references to
// #/components/schemas are kept as named links (Schema.Ref) and never inlined,
// because component schemas may be recursive.
package openapi

// Diagnostic is one problem found while reading a document. Diagnostics never
// stop the read; the affected value is skipped or kept as-is.
type Diagnostic struct {
	// Path is an RFC 6901 pointer into the document, e.g. "#/paths/~1pets/get/responses".
	Path string
	// Message describes the problem in plain words.
	Message string
}

// String renders the diagnostic as "path: message".
func (d Diagnostic) String() string { return d.Path + ": " + d.Message }

// Spec is the render-ready view of a whole OpenAPI document.
type Spec struct {
	Version     string // the "openapi" field, e.g. "3.0.3"
	Title       string // info.title
	APIVersion  string // info.version
	Summary     string // info.summary (3.1)
	Description string // info.description
	Servers     []Server
	// Tags lists declared tags in declared order, then tags used by operations
	// but not declared (first-use order), then "default" when any operation
	// has no tags.
	Tags []Tag
	// Operations are in document order: paths as written, methods in the
	// order get, put, post, delete, options, head, patch, trace.
	Operations      []*Operation
	Schemas         []*NamedSchema   // components.schemas in document order
	Security        []Requirement    // top-level security; nil when absent
	SecuritySchemes []SecurityScheme // components.securitySchemes in document order
}

// Server is one entry of a servers list.
type Server struct {
	URL         string
	Description string
	Variables   map[string]ServerVariable // nil when the server has none
}

// ServerVariable is a substitution variable of a server URL template.
type ServerVariable struct {
	Default     string
	Enum        []string
	Description string
}

// Tag groups operations; Description is empty for undeclared tags.
type Tag struct {
	Name        string
	Description string
}

// Operation is one HTTP method on one path.
type Operation struct {
	// ID is the operationId, or a slug derived from method and path
	// ("get-pets-petId"). IDs are unique within a Spec; collisions get -2, -3...
	ID          string
	Method      string // upper case, e.g. "GET"
	Path        string // the path template, e.g. "/pets/{petId}"
	Summary     string
	Description string
	Tags        []string // never empty: untagged operations get ["default"]
	Deprecated  bool
	// Parameters are path-item parameters merged with operation parameters;
	// an operation parameter overrides one with the same name and location.
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   []Response // document order; "default" kept as Status "default"
	// Security is the operation's own list, or the top-level one when the
	// operation declares none. An explicit empty list is a non-nil empty
	// slice and means "no authentication".
	Security []Requirement
	// Servers are operation- or path-level servers; nil means the Spec's.
	Servers []Server
}

// Parameter is a resolved operation parameter.
type Parameter struct {
	Name        string
	In          string // path, query, header or cookie
	Description string
	Required    bool
	Deprecated  bool
	Schema      *Schema
	Example     any // JSON-friendly: maps are map[string]any
}

// RequestBody is a resolved request body.
type RequestBody struct {
	Description string
	Required    bool
	Content     []MediaType // document order
}

// MediaType is one entry of a content map.
type MediaType struct {
	Type   string // e.g. "application/json"
	Schema *Schema
	// Example is "example", or the value of the first of "examples"; else nil.
	Example any
}

// Response is one resolved response of an operation.
type Response struct {
	Status      string // "200", "4XX" or "default"
	Description string
	Content     []MediaType
	Headers     []Header
}

// Header is one resolved response header.
type Header struct {
	Name        string
	Description string
	Schema      *Schema
}

// Requirement is one security requirement object: all of its items apply
// together (AND). A list of Requirements is alternatives (OR). An empty
// Requirement means anonymous access is allowed.
type Requirement []RequirementItem

// RequirementItem names one security scheme and the scopes it needs.
type RequirementItem struct {
	Scheme string
	Scopes []string
}

// SecurityScheme is one entry of components.securitySchemes.
type SecurityScheme struct {
	Name         string // key in components.securitySchemes
	Type         string // apiKey, http, oauth2, openIdConnect, mutualTLS
	Description  string
	In           string // apiKey: header, query or cookie
	ParamName    string // apiKey: the header, query or cookie name
	Scheme       string // http: "bearer", "basic", ...
	BearerFormat string // http bearer: a hint such as "JWT"
}

// NamedSchema is one entry of components.schemas.
type NamedSchema struct {
	Name   string
	Schema *Schema
}
