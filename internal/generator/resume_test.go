package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/resume"
)

// resumeDoc is a CV with every section the structured data maps.
const resumeDoc = `{
  "$schema": "https://schema-resume.org/1.3/schema.json",
  "basics": {"name": "Ada Example", "label": "Engineer", "title": "Dr.", "summary": "Builds things.",
    "email": "ada@example.com", "phone": "+1 555 0100", "url": "https://example.com/", "image": "https://example.com/ada.jpg",
    "location": {"streetAddress": "1 Main St\nFlat 2", "postalCode": "00-001", "city": "Springfield", "region": "North", "countryCode": "EX"},
    "profiles": [{"network": "Code", "url": "https://code.example.com/ada"}, {"network": "Bad", "url": "/relative"}]},
  "work": [
    {"name": "Acme", "url": "https://acme.example.com", "position": "Lead", "startDate": "2021-06"},
    {"name": "Acme", "position": "Lead", "startDate": "2020", "endDate": "2021-05"},
    {"name": "Initech", "positions": [{"position": "Dev", "startDate": "2015", "endDate": "2017"}, {"position": "Senior Dev", "startDate": "2018"}]},
    {"name": "", "position": ""}
  ],
  "education": [{"institution": "Example University", "url": "https://uni.example.com"}, {"school": "Example University"}, {}],
  "certificates": [{"name": "CKA", "issuer": "Board", "date": "2022-05", "url": "https://certs.example.com"}, {"name": "Plain", "date": "soon"}, {"issuer": "Nameless"}],
  "skills": [{"name": "Go", "keywords": ["Concurrency", "Go"]}, {"name": "Ops"}],
  "languages": [{"language": "Polish"}, {"language": "Polish"}, {"language": ""}],
  "awards": [{"title": "Best"}, {"title": " "}],
  "meta": {"dateCreated": "2026-01-10T09:00:00Z", "dateModified": "2026-10-01T12:00:00Z"},
  "custom": {"kept": true}
}`

// writeResume puts a document in a temp dir and returns its path.
func writeResume(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cv.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResumeURL(t *testing.T) {
	for in, want := range map[string]string{"": "/cv/", "  ": "/cv/", "/": "/", "cv": "/cv/", "/about/me": "/about/me/", "/x/": "/x/"} {
		if got := resumeURL(in); got != want {
			t.Errorf("resumeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResumeLD(t *testing.T) {
	doc, err := resume.Parse([]byte(resumeDoc))
	if err != nil {
		t.Fatal(err)
	}
	ld := resumeLD(doc, false)
	if ld["@type"] != "ProfilePage" || ld["dateModified"] != "2026-10-01T12:00:00Z" || ld["dateCreated"] != "2026-01-10T09:00:00Z" {
		t.Errorf("page = %v", ld)
	}
	person := ld["mainEntity"].(map[string]interface{})
	got, _ := json.Marshal(person)
	for _, want := range []string{
		`"@type":"Person"`, `"name":"Ada Example"`, `"jobTitle":"Engineer"`, `"honorificPrefix":"Dr."`,
		`"url":"https://example.com/"`, `"image":"https://example.com/ada.jpg"`, `"description":"Builds things."`,
		`"sameAs":["https://code.example.com/ada"]`,
		`"worksFor":[{"@type":"Organization","name":"Acme","url":"https://acme.example.com"},{"@type":"Organization","name":"Initech"}]`,
		`"hasOccupation":[{"@type":"Occupation","name":"Lead"},{"@type":"Occupation","name":"Dev"},{"@type":"Occupation","name":"Senior Dev"}]`,
		`"alumniOf":[{"@type":"EducationalOrganization","name":"Example University","url":"https://uni.example.com"}]`,
		`{"@type":"EducationalOccupationalCredential","dateCreated":"2022-05","name":"CKA","recognizedBy":{"@type":"Organization","name":"Board"},"url":"https://certs.example.com"}`,
		`{"@type":"EducationalOccupationalCredential","name":"Plain"}`,
		`"knowsAbout":["Go","Concurrency","Ops"]`, `"knowsLanguage":[{"@type":"Language","name":"Polish"}]`, `"award":["Best"]`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("Person lacks %s:\n%s", want, got)
		}
	}
	for _, leak := range []string{"email", "telephone", "address", "Nameless"} {
		if strings.Contains(string(got), leak) {
			t.Errorf("Person must not carry %s without contact", leak)
		}
	}
	withContact, _ := json.Marshal(resumeLD(doc, true)["mainEntity"])
	for _, want := range []string{`"email":"ada@example.com"`, `"telephone":"+1 555 0100"`,
		`"address":{"@type":"PostalAddress","addressCountry":"EX","addressLocality":"Springfield","addressRegion":"North","postalCode":"00-001","streetAddress":"1 Main St, Flat 2"}`} {
		if !strings.Contains(string(withContact), want) {
			t.Errorf("contact Person lacks %s:\n%s", want, withContact)
		}
	}
}

func TestResumeLDMinimal(t *testing.T) {
	ld := resumeLD(&resume.Resume{Basics: resume.Basics{Name: "N", URL: "not a url"}}, true)
	person := ld["mainEntity"].(map[string]interface{})
	if len(person) != 2 || ld["dateCreated"] != nil {
		t.Errorf("a bare document is a bare Person: %v", ld)
	}
	if postalAddress(&resume.Location{}) != nil || postalAddress(nil) != nil {
		t.Error("an empty location is no address")
	}
	if a := postalAddress(&resume.Location{Address: "Street 1", Country: "Exland"}); a["streetAddress"] != "Street 1" || a["addressCountry"] != "Exland" {
		t.Errorf("address fallbacks: %v", a)
	}
}

// buildResumeSite builds a one-page site with the résumé and returns the
// output directory.
func buildResumeSite(t *testing.T, opt ResumeOptions) (*Generator, string) {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "pages", "about.md"), "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nSee the [CV]("+resumeURL(opt.URL)+") and [its data]("+resumeURL(opt.URL)+"resume.json).\n")
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: out, Quiet: true, SEO: true,
		CheckLinks: "strict", CheckSchema: "strict", Resume: opt})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return gen, out
}

func TestResumeBuild(t *testing.T) {
	gen, out := buildResumeSite(t, ResumeOptions{File: writeResume(t, resumeDoc)})
	page := readFileString(t, filepath.Join(out, "cv", "index.html"))
	for _, want := range []string{`<link rel="alternate" type="application/json" href="/cv/resume.json">`,
		"<h2 id=\"experience\">Experience</h2>", `"@type":"ProfilePage"`, `"mainEntity":{"@type":"Person"`, "Ada Example"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if n := len(regexp.MustCompile(`"@type":"ProfilePage"`).FindAllString(page, -1)); n != 1 {
		t.Errorf("one ProfilePage block, got %d", n)
	}
	published := readFileString(t, filepath.Join(out, "cv", "resume.json"))
	for _, leak := range []string{"ada@example.com", "555 0100", "Main St", "00-001"} {
		if strings.Contains(page, leak) || strings.Contains(published, leak) {
			t.Errorf("%q published without contact", leak)
		}
	}
	if !strings.Contains(published, `"kept": true`) || !strings.Contains(published, "Springfield") {
		t.Errorf("resume.json keeps the rest:\n%s", published)
	}
	if gen.resume == nil || gen.resume.url != "/cv/" {
		t.Error("the build remembers the résumé")
	}
	var p *models.Page
	for i := range gen.siteData.Pages {
		if gen.siteData.Pages[i].Layout == resumeLayout {
			p = &gen.siteData.Pages[i]
		}
	}
	if p == nil || p.Title != "Ada Example" || p.Description != "Builds things." || p.Modified.IsZero() ||
		p.Extra["resume_json"] != "/cv/resume.json" || p.Extra["resume"].(*resume.Resume).Basics.Email != "" {
		t.Errorf("page = %+v", p)
	}
}

func TestResumeBuildWithContact(t *testing.T) {
	path := writeResume(t, resumeDoc)
	_, out := buildResumeSite(t, ResumeOptions{File: path, URL: "/about/me", Title: "Curriculum vitae", Contact: true})
	page := readFileString(t, filepath.Join(out, "about", "me", "index.html"))
	for _, want := range []string{"Curriculum vitae", "mailto:ada@example.com", `"telephone":"+1 555 0100"`, `href="/about/me/resume.json"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	src, _ := os.ReadFile(path)
	if got := readFileString(t, filepath.Join(out, "about", "me", "resume.json")); got != string(src) {
		t.Error("with contact, resume.json is the document as written")
	}
}

func TestResumeErrorsAndNoop(t *testing.T) {
	g := &Generator{config: Config{}}
	if err := g.loadResume(); err != nil || g.resume != nil || g.writeResumeJSON() != nil {
		t.Error("no resume.file means nothing to do")
	}
	if g.resumeAlternate(models.Page{Link: "/cv/"}, "") != "" {
		t.Error("no résumé, no alternate")
	}
	g = &Generator{config: Config{Quiet: true, Resume: ResumeOptions{File: filepath.Join(t.TempDir(), "missing.json")}}}
	if err := g.loadResume(); err == nil {
		t.Error("a missing document fails the build")
	}
	g.config.Resume.File = writeResume(t, `{"basics": {"label": "x"}, "work": [{"startDate": "2020-13"}]}`)
	if err := g.loadResume(); err == nil || !strings.Contains(err.Error(), "basics.name") {
		t.Errorf("a nameless document fails the build: %v", err)
	}
	g.config.Resume.File = writeResume(t, `{"basics": {"name": "N", "email": "n@example.com"}, "bad": 1e400}`)
	if err := g.loadResume(); err == nil {
		t.Error("an undecodable document fails the build")
	}
}

func TestResumeAlternateAndWrite(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir()}, resume: &resumeSite{url: "/cv/", json: []byte("{}")}}
	if got := g.resumeAlternate(models.Page{Link: "/cv/"}, ""); !strings.Contains(got, `href="/cv/resume.json"`) {
		t.Errorf("alternate = %q", got)
	}
	if g.resumeAlternate(models.Page{Link: "/cv/"}, `<link href="/cv/resume.json">`) != "" || g.resumeAlternate(models.Page{Link: "/other/"}, "") != "" {
		t.Error("only the résumé page, and only once")
	}
	if err := g.writeResumeJSON(); err != nil {
		t.Fatal(err)
	}
	g.resume.url = "/../../escape/"
	if err := g.writeResumeJSON(); err == nil {
		t.Error("a URL outside the output is refused")
	}
	blocker := filepath.Join(g.config.OutputDir, "file")
	_ = os.WriteFile(blocker, nil, 0o600)
	g.resume.url = "/file/"
	if err := g.writeResumeJSON(); err == nil {
		t.Error("an unwritable directory is an error")
	}
}

func TestResumeModified(t *testing.T) {
	if !resumeModified(resume.Meta{LastModified: "2025-10-09T12:47:00+01:00"}).Equal(resumeModified(resume.Meta{DateModified: "2025-10-09T11:47:00Z"})) {
		t.Error("lastModified is the fallback for dateModified")
	}
	if !resumeModified(resume.Meta{LastModified: "2025"}).IsZero() {
		t.Error("a date without a time is not a modification time")
	}
	p := resumePage(&resume.Resume{Basics: resume.Basics{Name: "N", Label: "L"}}, ResumeOptions{}, "")
	if p.Description != "L" || p.Link != "/cv/" || p.Slug != "cv" {
		t.Errorf("page = %+v", p)
	}
}
