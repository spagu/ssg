// Package apisource holds what the code extractors share (GO-104, GO-105):
// their configuration and the diagnostics they report. Each language lives
// in a subpackage — js for JavaScript with comments, dts for TypeScript
// declaration files — and fills an apimodel.Package.
package apisource

// Config is one package to document, from the api_docs configuration.
type Config struct {
	Name    string   // package name, the first segment of every ID
	Root    string   // directory holding package.json and the sources
	Entries []string // entry files relative to Root; empty = from package.json
	Include []string // globs relative to Root; empty = everything reachable from the entries
	Exclude []string // globs relative to Root
}

// Severity says whether a diagnostic stops a strict build.
type Severity string

// Severities.
const (
	Warning Severity = "warning"
	Error   Severity = "error"
)

// Diagnostic is one thing an extractor could not read or understand,
// located at a file and line.
type Diagnostic struct {
	Severity Severity
	File     string // relative to Root
	Line     int
	Message  string
}

func (d Diagnostic) String() string {
	return d.File + ":" + itoa(d.Line) + ": " + d.Message
}

// itoa avoids strconv for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
