package resume

import (
	"strconv"
	"strings"
)

// Markdown renders the document as the body of a page: plain Markdown, so
// it reads the same in any theme. The person's name is left to the page
// title. Contact adds the e-mail, phone and street address; without it the
// page names only the city, region and country.
func Markdown(r *Resume, contact bool) string {
	m := &mdWriter{}
	writeIntro(m, r.Basics, contact)
	m.list("Key achievements", r.Basics.KeyAchievements)
	m.list("Core competencies", r.Basics.CoreCompetencies)
	writeWork(m, r.Work)
	writeVolunteer(m, r.Volunteer)
	writeEducation(m, r.Education)
	writeSkills(m, r.Skills, r.Tools)
	writeCredentials(m, r)
	writeProjects(m, r.Projects)
	writeExtras(m, r)
	return strings.TrimSpace(m.String()) + "\n"
}

// mdWriter builds Markdown block by block, one blank line between blocks.
type mdWriter struct{ strings.Builder }

// block writes a paragraph-level block; empty text writes nothing.
func (m *mdWriter) block(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	_, _ = m.WriteString(text)
	_, _ = m.WriteString("\n\n")
}

// heading writes a section or entry heading.
func (m *mdWriter) heading(level int, text string) {
	m.block(strings.Repeat("#", level) + " " + text)
}

// bullets writes a list of already-formatted items.
func (m *mdWriter) bullets(items []string) {
	var b strings.Builder
	for _, it := range items {
		if strings.TrimSpace(it) != "" {
			b.WriteString("- " + strings.ReplaceAll(it, "\n", " ") + "\n")
		}
	}
	m.block(strings.TrimSuffix(b.String(), "\n"))
}

// list writes a titled list of plain strings.
func (m *mdWriter) list(title string, items []string) {
	if len(items) == 0 {
		return
	}
	m.heading(2, Escape(title))
	escaped := make([]string, len(items))
	for i, s := range items {
		escaped[i] = Escape(s)
	}
	m.bullets(escaped)
}

// joinNonEmpty joins the non-blank parts.
func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// emphasis wraps non-empty text in italics.
func emphasis(s string) string {
	if s == "" {
		return ""
	}
	return "*" + s + "*"
}

// years is "6 years" for a yearsOfExperience.
func years(n float64) string {
	if n <= 0 {
		return ""
	}
	unit := " years"
	if n == 1 {
		unit = " year"
	}
	return strconv.FormatFloat(n, 'f', -1, 64) + unit
}

// escapeAll escapes every string of a list.
func escapeAll(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if e := Escape(s); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// place is a location as a line: street parts only with contact.
func place(l *Location, contact bool) string {
	if l == nil {
		return ""
	}
	country := l.Country
	if country == "" {
		country = l.CountryCode
	}
	parts := []string{l.City, l.Region, country}
	if contact {
		parts = append([]string{strings.ReplaceAll(joinNonEmpty(", ", l.StreetAddress, l.Address), "\n", ", "), l.PostalCode}, parts...)
	}
	return Escape(joinNonEmpty(", ", parts...))
}

// writeIntro is the label, the summary, the links and — with contact — the
// ways to reach the person.
func writeIntro(m *mdWriter, b Basics, contact bool) {
	m.block(bold(Escape(b.Label)))
	m.block(Escape(b.Summary))
	var lines []string
	if contact {
		if b.Email != "" {
			lines = append(lines, "E-mail: ["+Escape(b.Email)+"](mailto:"+urlEscapes.Replace(b.Email)+")")
		}
		if b.Phone != "" {
			lines = append(lines, "Phone: "+Escape(b.Phone))
		}
	}
	if p := place(b.Location, contact); p != "" {
		lines = append(lines, "Location: "+p)
	}
	if WebURL(b.URL) {
		lines = append(lines, "Website: "+Link(b.URL, b.URL))
	}
	for _, p := range b.Profiles {
		name := joinNonEmpty(" ", p.Network, paren(p.Username))
		if WebURL(p.URL) {
			lines = append(lines, Link(name, p.URL))
		}
	}
	m.bullets(lines)
}

// bold wraps already-escaped Markdown in strong emphasis, or is nothing.
func bold(md string) string {
	if md == "" {
		return ""
	}
	return "**" + md + "**"
}

// paren is "(s)", or nothing.
func paren(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "(" + s + ")"
}
