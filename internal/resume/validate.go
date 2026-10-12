package resume

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

// Severity of a diagnostic: an Error means the document cannot be published
// as it stands; a Warning means a part of it will be wrong or left out.
type Severity string

// Severities.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Diagnostic is one problem, at a path such as "work[2].startDate".
type Diagnostic struct {
	Severity Severity
	Path     string
	Message  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s: %s", d.Severity, d.Path, d.Message)
}

// HasErrors reports whether any diagnostic is an Error.
func HasErrors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// checker collects diagnostics.
type checker struct{ out []Diagnostic }

func (c *checker) add(sev Severity, path, format string, args ...interface{}) {
	c.out = append(c.out, Diagnostic{Severity: sev, Path: path, Message: fmt.Sprintf(format, args...)})
}

// date checks one date field.
func (c *checker) date(path string, d Date) {
	if d != "" && !d.Valid() {
		c.add(Warning, path, "%q is not an ISO 8601 date (YYYY, YYYY-MM or YYYY-MM-DD)", string(d))
	}
}

// span checks a start/end pair: both well formed, end not before start.
func (c *checker) span(path string, start, end Date) {
	c.date(path+".startDate", start)
	c.date(path+".endDate", end)
	if start.Valid() && end.Valid() && end.Before(start) {
		c.add(Warning, path+".endDate", "%s is before the start date %s", end, start)
	}
}

// url checks a link: absolute, and http or https.
func (c *checker) url(path, raw string) {
	if raw == "" {
		return
	}
	if !WebURL(raw) {
		c.add(Warning, path, "%q is not an absolute http(s) URL", raw)
	}
}

// WebURL reports whether s is an absolute http or https URL with a host —
// the only links ssg writes into the page or the structured data.
func WebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// email checks an address.
func (c *checker) email(path, raw string) {
	if raw == "" {
		return
	}
	if _, err := mail.ParseAddress(raw); err != nil || strings.ContainsAny(raw, "<> ") {
		c.add(Warning, path, "%q is not an e-mail address", raw)
	}
}

// timestamp checks a meta date-time.
func (c *checker) timestamp(path, raw string) {
	if raw == "" {
		return
	}
	if _, err := time.Parse(time.RFC3339, raw); err != nil && !Date(raw).Valid() {
		c.add(Warning, path, "%q is not an ISO 8601 date-time", raw)
	}
}

// Validate checks a document against the rules the schema states.
func Validate(r *Resume) []Diagnostic {
	c := &checker{}
	if s := r.Schema; s != "" && !strings.Contains(s, "schema-resume.org") {
		c.add(Warning, "$schema", "%q is not a schema-resume.org schema; fields may be read differently", s)
	}
	checkBasics(c, r.Basics)
	for i, w := range r.Work {
		p := fmt.Sprintf("work[%d]", i)
		c.span(p, w.StartDate, w.EndDate)
		c.url(p+".url", w.URL)
		if w.ContactDetails != nil {
			c.email(p+".contactDetails.email", w.ContactDetails.Email)
		}
		for j, pos := range w.Positions {
			c.span(fmt.Sprintf("%s.positions[%d]", p, j), pos.StartDate, pos.EndDate)
		}
	}
	for i, v := range r.Volunteer {
		p := fmt.Sprintf("volunteer[%d]", i)
		c.span(p, v.StartDate, v.EndDate)
		c.url(p+".url", v.URL)
	}
	for i, e := range r.Education {
		p := fmt.Sprintf("education[%d]", i)
		c.span(p, e.StartDate, e.EndDate)
		c.url(p+".url", e.URL)
	}
	checkDated(c, r)
	checkMeta(c, r.Meta)
	return c.out
}

// checkBasics checks the person.
func checkBasics(c *checker, b Basics) {
	if strings.TrimSpace(b.Name) == "" {
		c.add(Error, "basics.name", "the person's name is required")
	}
	c.date("basics.dateOfBirth", b.DateOfBirth)
	c.email("basics.email", b.Email)
	c.url("basics.url", b.URL)
	for i, p := range b.Profiles {
		c.url(fmt.Sprintf("basics.profiles[%d].url", i), p.URL)
	}
	if b.LegalNote != nil {
		c.url("basics.legalNote.url", b.LegalNote.URL)
	}
	for i, w := range b.WorkAuthorization {
		c.span(fmt.Sprintf("basics.workAuthorization[%d]", i), w.ValidFrom, w.ValidTo)
	}
}

// checkDated checks the sections whose entries carry one date and a link.
func checkDated(c *checker, r *Resume) {
	for i, a := range r.Awards {
		c.date(fmt.Sprintf("awards[%d].date", i), a.Date)
	}
	for i, x := range r.Certificates {
		c.date(fmt.Sprintf("certificates[%d].date", i), x.Date)
		c.url(fmt.Sprintf("certificates[%d].url", i), x.URL)
	}
	for i, x := range r.Publications {
		c.date(fmt.Sprintf("publications[%d].releaseDate", i), x.ReleaseDate)
		c.url(fmt.Sprintf("publications[%d].url", i), x.URL)
	}
	for i, x := range r.Tools {
		c.url(fmt.Sprintf("tools[%d].url", i), x.URL)
	}
	for i, x := range r.Projects {
		c.span(fmt.Sprintf("projects[%d]", i), x.StartDate, x.EndDate)
		c.url(fmt.Sprintf("projects[%d].url", i), x.URL)
	}
}

// checkMeta checks the document's own metadata.
func checkMeta(c *checker, m Meta) {
	c.url("meta.canonical", m.Canonical)
	c.timestamp("meta.lastModified", m.LastModified)
	c.timestamp("meta.dateCreated", m.DateCreated)
	c.timestamp("meta.dateModified", m.DateModified)
	c.timestamp("meta.datePublished", m.DatePublished)
}
