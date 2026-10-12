package resume

import (
	"strings"
	"testing"
)

// fullDoc fills every section the Markdown writes.
const fullDoc = `{
  "basics": {"name": "Ada", "label": "Engineer", "summary": "Builds *things*.", "email": "ada@example.com",
    "phone": "+1 555", "url": "https://example.com",
    "location": {"address": "1 Main St\nFlat 2", "postalCode": "00-001", "city": "Springfield", "country": "Exland"},
    "profiles": [{"network": "Code", "username": "ada", "url": "https://code.example.com/ada"}, {"network": "Bad", "url": "javascript:x"}],
    "keyAchievements": ["Won"], "coreCompetencies": ["Ops"]},
  "work": [
    {"name": "Acme", "url": "https://acme.example.com", "position": "Lead", "startDate": "2021-06", "workLocation": "Remote",
     "workType": "full-time", "summary": "Leads.", "highlights": ["Led\nwell"]},
    {"name": "Initech", "startDate": "2015", "endDate": "2020", "summary": "Grew.", "positions": [
      {"position": "Dev", "workType": "contract", "startDate": "2015", "endDate": "2017", "summary": "Coded.", "highlights": ["Shipped"]}]}
  ],
  "volunteer": [{"organization": "Club", "position": "Mentor", "startDate": "2019", "summary": "Teaches.", "highlights": ["Taught"]}],
  "education": [
    {"institution": "Uni", "degree": "BSc CS", "startDate": "2011", "endDate": "2015", "score": "First", "courses": ["Algo"], "description": "Studied."},
    {"school": "High", "studyType": "A-Levels", "area": "Maths", "gpa": "4.0", "location": "Town", "country": "EX"}
  ],
  "skills": [{"name": "Go", "level": "Expert", "yearsOfExperience": 1, "keywords": ["C#"], "comment": "Daily"}],
  "tools": [{"name": "Kubernetes", "url": "https://k8s.example.com", "level": "Advanced", "yearsOfExperience": 2.5, "group": "Ops"}, {"name": "Vim", "category": "Editors"}],
  "certificates": [{"name": "CKA", "issuer": "Board", "date": "2022-05", "url": "https://certs.example.com"}],
  "languages": [{"language": "Polish", "fluency": "Native"}],
  "projects": [{"name": "tool", "url": "https://tool.example.com", "startDate": "2022", "roles": ["Author"], "entity": "Me",
    "description": "A tool.", "highlights": ["Stars"], "keywords": ["Go"]}],
  "publications": [{"name": "Post", "url": "https://blog.example.com/post", "publisher": "Blog", "releaseDate": "2024-02-14", "summary": "On things."}],
  "awards": [{"title": "Best", "awarder": "Acme", "date": "2023", "summary": "For being best."}],
  "interests": [{"name": "Hiking", "keywords": ["Alps"]}],
  "references": [{"name": "Bob", "reference": "Great.\nReally."}, {"name": "Silent"}]
}`

func TestMarkdownSections(t *testing.T) {
	r, err := Parse([]byte(fullDoc))
	if err != nil {
		t.Fatal(err)
	}
	md := Markdown(r, false)
	for _, want := range []string{
		"**Engineer**", `Builds \*things\*.`, "- Location: Springfield, Exland", "- Website: [https://example.com](https://example.com)",
		"- [Code (ada)](https://code.example.com/ada)", "## Key achievements\n\n- Won", "## Core competencies\n\n- Ops",
		"## Experience", "### Lead — [Acme](https://acme.example.com)", "*2021-06 – present · Remote · full-time*", "- Led well",
		"### Initech\n", "*2015 – 2020*", "#### Dev", "*2015 – 2017 · contract*", "- Shipped",
		"## Volunteering", "### Mentor — Club", "*2019 – present*", "- Taught",
		"## Education", "### BSc CS — Uni", "*2011 – 2015 · First*", "Courses: Algo", "Studied.",
		"### A-Levels, Maths — High", "*Town, EX · GPA 4.0*",
		"## Skills", `- **Go** (Expert, 1 year): C\# — Daily`,
		"## Tools", "- [Kubernetes](https://k8s.example.com) (Advanced, 2.5 years, Ops)", "- Vim (Editors)",
		"## Certificates", "- [CKA](https://certs.example.com) — Board, 2022-05",
		"## Languages", "- Polish — Native",
		"## Projects", "### [tool](https://tool.example.com)", "*2022 – present · Author · Me*", "Keywords: Go",
		"## Publications", "**[Post](https://blog.example.com/post)** — Blog, 2024-02-14", "On things.",
		"## Awards", "**Best** — Acme, 2023", "For being best.",
		"## Interests", "- **Hiking**: Alps",
		"## References", "> Great.\n> Really.\n>\n> — Bob",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q:\n%s", want, md)
		}
	}
	for _, leak := range []string{"ada@example.com", "+1 555", "Main St", "00-001", "javascript", "Silent"} {
		if strings.Contains(md, leak) {
			t.Errorf("Markdown must not contain %q without contact", leak)
		}
	}
	contact := Markdown(r, true)
	for _, want := range []string{"- E-mail: [ada@example.com](mailto:ada@example.com)", `- Phone: \+1 555`,
		"- Location: 1 Main St, Flat 2, 00-001, Springfield, Exland"} {
		if !strings.Contains(contact, want) {
			t.Errorf("contact Markdown lacks %q:\n%s", want, contact)
		}
	}
}

func TestMarkdownEmptyDocument(t *testing.T) {
	if got := Markdown(&Resume{}, true); got != "\n" {
		t.Errorf("an empty document is an empty body, got %q", got)
	}
	r := &Resume{Basics: Basics{Location: &Location{CountryCode: "PL"}}, Work: []Work{{Name: "X"}}}
	if md := Markdown(r, false); !strings.Contains(md, "Location: PL") || !strings.Contains(md, "### X") {
		t.Errorf("got %q", md)
	}
}

func TestEscape(t *testing.T) {
	for in, want := range map[string]string{
		"C# & <b>":     `C\# & &lt;b&gt;`,
		"a_b*c`d[e]|~": `a\_b\*c\` + "`" + `d\[e\]\|\~`,
		"- item":       `\- item`,
		"+ item":       `\+ item`,
		"===":          `\===`,
		"1. first":     `1\. first`,
		"2) second":    `2\) second`,
		"2024":         "2024",
		"x\n  - y":     "x\n  \\- y",
		"x\n\ny":       "x\n\ny",
	} {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinkAndSpan(t *testing.T) {
	if got := Link("a (b)", "https://example.com/x (y)"); got != "[a (b)](https://example.com/x%20%28y%29)" {
		t.Errorf("Link = %q", got)
	}
	if Link("", "https://example.com") != "[https://example.com](https://example.com)" || Link("t", "/rel") != "t" || Link("", "") != "" {
		t.Error("Link fallbacks")
	}
	for _, c := range [][3]string{{"", "", ""}, {"", "2020", "2020"}, {"2020", "", "2020 – present"}, {"2019", "2020", "2019 – 2020"}} {
		if got := Span(Date(c[0]), Date(c[1])); got != c[2] {
			t.Errorf("Span(%q, %q) = %q", c[0], c[1], got)
		}
	}
	if years(0) != "" || years(1) != "1 year" || years(3) != "3 years" {
		t.Error("years")
	}
	if place(nil, true) != "" || firstOf("", " ") != "" || emphasis("") != "" || paren(" ") != "" {
		t.Error("empty helpers")
	}
}
