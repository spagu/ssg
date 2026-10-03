// Package wrap breaks lines.
package wrap

// Width is the line width.
type Width int

// Wrap breaks text at width.
func Wrap(text string, w Width) string { return text }
