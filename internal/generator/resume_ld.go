package generator

// The résumé's structured data: a ProfilePage whose mainEntity is the
// Person, built with the same personLD the author pages use and extended
// with what a CV knows — job title, employers, schools, credentials, skills,
// languages and awards. Contact details join only with resume.contact.

import (
	"strings"

	"github.com/spagu/ssg/internal/resume"
)

// resumeLD is the page's schema: ProfilePage > Person.
func resumeLD(doc *resume.Resume, contact bool) map[string]interface{} {
	b := doc.Basics
	var sameAs []string
	for _, p := range b.Profiles {
		if resume.WebURL(p.URL) {
			sameAs = append(sameAs, p.URL)
		}
	}
	person := personLD(strings.TrimSpace(b.Name), webURL(b.URL), b.Summary, webURL(b.Image), sameAs)
	setNonEmpty(person, "jobTitle", b.Label)
	setNonEmpty(person, "honorificPrefix", b.Title)
	setList(person, "worksFor", currentEmployers(doc.Work))
	setList(person, "hasOccupation", occupations(doc.Work))
	setList(person, "alumniOf", schools(doc.Education))
	setList(person, "hasCredential", credentials(doc.Certificates))
	setList(person, "knowsAbout", stringItems(knowledge(doc.Skills)))
	setList(person, "knowsLanguage", languages(doc.Languages))
	setList(person, "award", stringItems(awardTitles(doc.Awards)))
	if contact {
		setNonEmpty(person, "email", b.Email)
		setNonEmpty(person, "telephone", b.Phone)
		if addr := postalAddress(b.Location); addr != nil {
			person["address"] = addr
		}
	}
	ld := profilePageLD("", person)
	if m := doc.Meta; m.DateCreated != "" || m.DateModified != "" {
		setNonEmpty(ld, "dateCreated", m.DateCreated)
		setNonEmpty(ld, "dateModified", m.DateModified)
	}
	return ld
}

// webURL is s when it is an absolute http(s) URL, else "".
func webURL(s string) string {
	if resume.WebURL(strings.TrimSpace(s)) {
		return strings.TrimSpace(s)
	}
	return ""
}

// setList sets a key to a non-empty list.
func setList(m map[string]interface{}, key string, items []interface{}) {
	if len(items) > 0 {
		m[key] = items
	}
}

// organizationLD is a named organisation of a type, with its site if known.
func organizationLD(ldType, name, url string) map[string]interface{} {
	org := map[string]interface{}{"@type": ldType, "name": strings.TrimSpace(name)}
	setNonEmpty(org, "url", webURL(url))
	return org
}

// currentEmployers are the organisations of the open work entries.
func currentEmployers(work []resume.Work) []interface{} {
	var out []interface{}
	seen := map[string]bool{}
	for _, w := range work {
		name := strings.TrimSpace(w.Name)
		if name == "" || !w.Current() || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, organizationLD("Organization", name, w.URL))
	}
	return out
}

// occupations are the positions held, one Occupation per distinct title.
func occupations(work []resume.Work) []interface{} {
	var titles []string
	for _, w := range work {
		titles = append(titles, w.Position)
		for _, p := range w.Positions {
			titles = append(titles, p.Position)
		}
	}
	var out []interface{}
	for _, t := range uniqueStrings(trimAll(titles)) {
		out = append(out, map[string]interface{}{"@type": "Occupation", "name": t})
	}
	return out
}

// schools are the educational organisations, once each.
func schools(edu []resume.Education) []interface{} {
	var out []interface{}
	seen := map[string]bool{}
	for _, e := range edu {
		name := strings.TrimSpace(e.Organization())
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, organizationLD("EducationalOrganization", name, e.URL))
	}
	return out
}

// credentials are the certificates, each recognised by its issuer.
func credentials(certs []resume.Certificate) []interface{} {
	var out []interface{}
	for _, c := range certs {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		cred := map[string]interface{}{"@type": "EducationalOccupationalCredential", "name": strings.TrimSpace(c.Name)}
		setNonEmpty(cred, "url", webURL(c.URL))
		if c.Date.Valid() {
			cred["dateCreated"] = string(c.Date)
		}
		if strings.TrimSpace(c.Issuer) != "" {
			cred["recognizedBy"] = organizationLD("Organization", c.Issuer, "")
		}
		out = append(out, cred)
	}
	return out
}

// knowledge is every skill name and keyword, once each.
func knowledge(skills []resume.Skill) []string {
	var all []string
	for _, s := range skills {
		all = append(all, s.Name)
		all = append(all, s.Keywords...)
	}
	return uniqueStrings(trimAll(all))
}

// languages are the spoken languages as Language objects.
func languages(langs []resume.Language) []interface{} {
	var names []string
	for _, l := range langs {
		names = append(names, l.Language)
	}
	var out []interface{}
	for _, n := range uniqueStrings(trimAll(names)) {
		out = append(out, map[string]interface{}{"@type": "Language", "name": n})
	}
	return out
}

// awardTitles are the awards' titles.
func awardTitles(awards []resume.Award) []string {
	var titles []string
	for _, a := range awards {
		titles = append(titles, a.Title)
	}
	return uniqueStrings(trimAll(titles))
}

// postalAddress is the person's location as a PostalAddress.
func postalAddress(l *resume.Location) map[string]interface{} {
	if l == nil {
		return nil
	}
	addr := map[string]interface{}{"@type": "PostalAddress"}
	street := l.StreetAddress
	if street == "" {
		street = l.Address
	}
	setNonEmpty(addr, "streetAddress", strings.ReplaceAll(strings.TrimSpace(street), "\n", ", "))
	setNonEmpty(addr, "postalCode", l.PostalCode)
	setNonEmpty(addr, "addressLocality", l.City)
	setNonEmpty(addr, "addressRegion", l.Region)
	country := l.CountryCode
	if country == "" {
		country = l.Country
	}
	setNonEmpty(addr, "addressCountry", country)
	if len(addr) == 1 {
		return nil
	}
	return addr
}

// trimAll trims every string and drops the blank ones.
func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// stringItems is a []string as JSON-LD list values.
func stringItems(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
