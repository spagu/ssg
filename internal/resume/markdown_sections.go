package resume

import "strings"

// writeWork is the Experience section: one heading per employer, and one
// per position where the entry lists several.
func writeWork(m *mdWriter, work []Work) {
	if len(work) == 0 {
		return
	}
	m.heading(2, "Experience")
	for _, w := range work {
		org := Link(w.Name, w.URL)
		if len(w.Positions) == 0 {
			m.heading(3, joinNonEmpty(" — ", Escape(w.Position), org))
			m.block(emphasis(joinNonEmpty(" · ", Span(w.StartDate, w.EndDate), Escape(w.WorkLocation), Escape(w.WorkType))))
			m.block(Escape(w.Summary))
			m.bullets(escapeAll(w.Highlights))
			continue
		}
		m.heading(3, org)
		m.block(emphasis(joinNonEmpty(" · ", Span(w.StartDate, w.EndDate), Escape(w.WorkLocation))))
		m.block(Escape(w.Summary))
		m.bullets(escapeAll(w.Highlights))
		for _, p := range w.Positions {
			m.heading(4, Escape(p.Position))
			m.block(emphasis(joinNonEmpty(" · ", Span(p.StartDate, p.EndDate), Escape(p.WorkType))))
			m.block(Escape(p.Summary))
			m.bullets(escapeAll(p.Highlights))
		}
	}
}

// writeVolunteer is the Volunteering section.
func writeVolunteer(m *mdWriter, vol []Volunteer) {
	if len(vol) == 0 {
		return
	}
	m.heading(2, "Volunteering")
	for _, v := range vol {
		m.heading(3, joinNonEmpty(" — ", Escape(v.Position), Link(v.Organization, v.URL)))
		m.block(emphasis(Span(v.StartDate, v.EndDate)))
		m.block(Escape(v.Summary))
		m.bullets(escapeAll(v.Highlights))
	}
}

// writeEducation is the Education section.
func writeEducation(m *mdWriter, edu []Education) {
	if len(edu) == 0 {
		return
	}
	m.heading(2, "Education")
	for _, e := range edu {
		study := e.Degree
		if study == "" {
			study = joinNonEmpty(", ", e.StudyType, e.Area)
		}
		m.heading(3, joinNonEmpty(" — ", Escape(study), Link(e.Organization(), e.URL)))
		score := e.Score
		if score == "" && e.GPA != "" {
			score = "GPA " + e.GPA
		}
		m.block(emphasis(joinNonEmpty(" · ", Span(e.StartDate, e.EndDate), Escape(joinNonEmpty(", ", e.Location, e.Country)), Escape(score))))
		m.block(Escape(e.Description))
		if len(e.Courses) > 0 {
			m.block("Courses: " + strings.Join(escapeAll(e.Courses), ", "))
		}
	}
}

// writeSkills is the Skills and Tools sections.
func writeSkills(m *mdWriter, skills []Skill, tools []Tool) {
	if len(skills) > 0 {
		m.heading(2, "Skills")
		items := make([]string, 0, len(skills))
		for _, s := range skills {
			item := joinNonEmpty(" ", bold(Escape(s.Name)), paren(joinNonEmpty(", ", Escape(s.Level), years(s.YearsOfExperience))))
			if kw := escapeAll(s.Keywords); len(kw) > 0 {
				item = joinNonEmpty(": ", item, strings.Join(kw, ", "))
			}
			items = append(items, joinNonEmpty(" — ", item, Escape(s.Comment)))
		}
		m.bullets(items)
	}
	if len(tools) > 0 {
		m.heading(2, "Tools")
		items := make([]string, 0, len(tools))
		for _, t := range tools {
			name := Link(t.Name, t.URL)
			items = append(items, joinNonEmpty(" ", name, paren(joinNonEmpty(", ", Escape(t.Level), years(t.YearsOfExperience), Escape(firstOf(t.Group, t.Category))))))
		}
		m.bullets(items)
	}
}

// writeCredentials is the Certificates and Languages sections.
func writeCredentials(m *mdWriter, r *Resume) {
	if len(r.Certificates) > 0 {
		m.heading(2, "Certificates")
		items := make([]string, 0, len(r.Certificates))
		for _, c := range r.Certificates {
			items = append(items, joinNonEmpty(" — ", Link(c.Name, c.URL), joinNonEmpty(", ", Escape(c.Issuer), string(c.Date))))
		}
		m.bullets(items)
	}
	if len(r.Languages) > 0 {
		m.heading(2, "Languages")
		items := make([]string, 0, len(r.Languages))
		for _, l := range r.Languages {
			items = append(items, joinNonEmpty(" — ", Escape(l.Language), Escape(l.Fluency)))
		}
		m.bullets(items)
	}
}

// writeProjects is the Projects section.
func writeProjects(m *mdWriter, projects []Project) {
	if len(projects) == 0 {
		return
	}
	m.heading(2, "Projects")
	for _, p := range projects {
		m.heading(3, Link(p.Name, p.URL))
		m.block(emphasis(joinNonEmpty(" · ", Span(p.StartDate, p.EndDate), strings.Join(escapeAll(p.Roles), ", "), Escape(p.Entity))))
		m.block(Escape(p.Description))
		m.bullets(escapeAll(p.Highlights))
		if kw := escapeAll(p.Keywords); len(kw) > 0 {
			m.block("Keywords: " + strings.Join(kw, ", "))
		}
	}
}

// writeExtras is Publications, Awards, Interests and References.
func writeExtras(m *mdWriter, r *Resume) {
	if len(r.Publications) > 0 {
		m.heading(2, "Publications")
		for _, p := range r.Publications {
			m.block(joinNonEmpty(" — ", bold(Link(p.Name, p.URL)), Escape(joinNonEmpty(", ", p.Publisher, string(p.ReleaseDate)))))
			m.block(Escape(p.Summary))
		}
	}
	if len(r.Awards) > 0 {
		m.heading(2, "Awards")
		for _, a := range r.Awards {
			m.block(joinNonEmpty(" — ", bold(Escape(a.Title)), Escape(joinNonEmpty(", ", a.Awarder, string(a.Date)))))
			m.block(Escape(a.Summary))
		}
	}
	if len(r.Interests) > 0 {
		m.heading(2, "Interests")
		items := make([]string, 0, len(r.Interests))
		for _, in := range r.Interests {
			items = append(items, joinNonEmpty(": ", bold(Escape(in.Name)), strings.Join(escapeAll(in.Keywords), ", ")))
		}
		m.bullets(items)
	}
	if len(r.References) > 0 {
		m.heading(2, "References")
		for _, ref := range r.References {
			if text := Escape(ref.Reference); text != "" {
				m.block(joinNonEmpty("\n>\n> — ", "> "+strings.ReplaceAll(text, "\n", "\n> "), Escape(ref.Name)))
			}
		}
	}
}

// firstOf is the first non-blank string.
func firstOf(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
