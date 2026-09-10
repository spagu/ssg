package generator

// Rendering a build profile: the table a person reads and the file CI keeps.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProfileFileName is where --profile=json writes the report.
//
// It goes to the working directory, not the output tree. A site's output is
// what gets deployed, and nobody asked to publish how long their build took —
// the same line GO-095 draws between the site's content and the project's own
// business.
const ProfileFileName = "build-profile.json"

// ProfileSchema versions the JSON report, so a consumer comparing two builds
// can tell a format change from a site change.
const ProfileSchema = 1

// ProfileReport is the JSON artifact.
type ProfileReport struct {
	Schema   int            `json:"schema"`
	Version  string         `json:"version,omitempty"`
	Time     time.Time      `json:"time"`
	TotalMs  float64        `json:"total_ms"`
	Steps    []StepTiming   `json:"steps"`
	Counters []Counter      `json:"counters"`
	Pages    []PageTiming   `json:"pages"`
	Notes    []string       `json:"notes,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// Report builds the JSON view. version is the ssg that produced it; at is the
// build's wall-clock stamp, taken by the caller so a test can fix it.
func (p *Profile) Report(version string, at time.Time) ProfileReport {
	return ProfileReport{
		Schema:   ProfileSchema,
		Version:  version,
		Time:     at.UTC(),
		TotalMs:  millis(p.Total()),
		Steps:    p.Steps(),
		Counters: p.Counters(),
		Pages:    p.Pages(),
	}
}

// WriteJSON writes the report next to the project, and returns the path.
func (p *Profile) WriteJSON(dir, version string, at time.Time) (string, error) {
	if p == nil {
		return "", nil
	}
	data, err := json.MarshalIndent(p.Report(version, at), "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ProfileFileName)
	// #nosec G306 -- a build report beside the project, readable like the logs
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// LoadProfileReport reads a report written by WriteJSON.
func LoadProfileReport(dir string) (ProfileReport, error) {
	var r ProfileReport
	data, err := os.ReadFile(filepath.Join(dir, ProfileFileName)) // #nosec G304 -- the build's own report
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("%s: %w", ProfileFileName, err)
	}
	if r.Schema != ProfileSchema {
		return r, fmt.Errorf("%s: schema %d, this build reads %d", ProfileFileName, r.Schema, ProfileSchema)
	}
	return r, nil
}

// topPages is how many of the slowest pages the text report names. The full
// list is in the JSON, which is where a hunt for the tail belongs.
const topPages = 10

// WriteText prints the human report: phases in the order they ran, the
// counters the build kept, and the slowest pages.
func (p *Profile) WriteText(w io.Writer, at time.Time) {
	if p == nil {
		return
	}
	steps := p.Steps()
	total := p.Total()
	width := len("Total")
	for _, s := range steps {
		if len(s.Name) > width {
			width = len(s.Name)
		}
	}
	// A report is written to a terminal; a short write there is not something
	// the build can act on, so the error is dropped once, here, rather than at
	// every line.
	line := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

	line("\n⏱️  Build profile (%s)\n", at.Format("2006-01-02 15:04:05"))
	line("   %-*s  %10s\n", width, "Total", humanDuration(total))
	for _, s := range steps {
		line("   %-*s  %10s  %5s\n", width, s.Name, humanDuration(s.Duration), percentOf(s.Duration, total))
	}
	if counters := p.Counters(); len(counters) > 0 {
		parts := make([]string, 0, len(counters))
		for _, c := range counters {
			parts = append(parts, fmt.Sprintf("%s %d", c.Name, c.Value))
		}
		line("   Counters: %s\n", strings.Join(parts, " · "))
	}
	pages := p.Pages()
	if len(pages) == 0 {
		return
	}
	shown := pages
	if len(shown) > topPages {
		shown = shown[:topPages]
	}
	line("   Slowest pages (%d of %d):\n", len(shown), len(pages))
	for _, pt := range shown {
		line("     %-*s  %10s\n", width, truncatePath(pt.Path, width), humanDuration(pt.Duration))
	}
}

// truncatePath keeps a long output path inside the report's column, cutting
// the middle so both the section and the page name stay readable.
func truncatePath(path string, width int) string {
	if len(path) <= width || width < 8 {
		return path
	}
	keep := (width - 1) / 2
	return path[:keep] + "…" + path[len(path)-(width-keep-1):]
}

// humanDuration renders a duration the way a build report should read: seconds
// once the number is big enough to matter, milliseconds below that.
func humanDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.0f ms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
	}
}

// percentOf is a phase's share of the build, blank when there is no total to
// divide by.
func percentOf(d, total time.Duration) string {
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", float64(d)/float64(total)*100)
}
