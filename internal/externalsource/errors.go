package externalsource

import "fmt"

// SourceError is the unified error model: every failure names the source, its
// type, the pipeline stage and the cause. Identifiers are safe to log (file
// paths now; later phases must redact URLs/DSNs before they reach here).
type SourceError struct {
	Source     string
	SourceType string
	Stage      string // "config", "read", "parse", "transform"
	Err        error
}

// Error formats the unified error message.
func (e *SourceError) Error() string {
	return fmt.Sprintf("external source %q (%s) failed at %s: %v", e.Source, e.SourceType, e.Stage, e.Err)
}

// Unwrap exposes the cause for errors.Is/As.
func (e *SourceError) Unwrap() error { return e.Err }

// fail wraps a cause into the unified error model.
func fail(src Source, stage string, err error) *SourceError {
	return &SourceError{Source: src.Name, SourceType: src.Type, Stage: stage, Err: err}
}
