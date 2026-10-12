package resume

import (
	"strings"
	"testing"
)

func TestValidateReportsEveryRule(t *testing.T) {
	r, err := Parse([]byte(`{
	  "$schema": "https://jsonresume.example.com/schema.json",
	  "basics": {"name": " ", "email": "not-an-address", "url": "example.com", "dateOfBirth": "1990-1-1",
	    "profiles": [{"url": "javascript:alert(1)"}], "legalNote": {"url": "/privacy"},
	    "workAuthorization": [{"validFrom": "2024", "validTo": "2023"}]},
	  "work": [{"startDate": "2020-05", "endDate": "2019", "url": "ftp://example.com",
	    "contactDetails": {"email": "Jo <jo@example.com>"}, "positions": [{"startDate": "x"}]}],
	  "volunteer": [{"endDate": "2020-00", "url": "http://"}],
	  "education": [{"startDate": "2010", "url": "https://ok.example.com", "endDate": "2009-12-31"}],
	  "awards": [{"date": "soon"}],
	  "certificates": [{"date": "2020-02-30", "url": "nope"}],
	  "publications": [{"releaseDate": "2020-1", "url": "mailto:a@example.com"}],
	  "tools": [{"url": "//example.com"}],
	  "projects": [{"startDate": "2022", "endDate": "2021", "url": "https://p.example.com"}],
	  "meta": {"canonical": "x", "lastModified": "yesterday", "dateCreated": "2024-01-01",
	    "dateModified": "2024-01-01T00:00:00Z", "datePublished": "2024-13-01T00:00:00Z"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Severity{}
	for _, d := range Validate(r) {
		got[d.Path] = d.Severity
		if !strings.Contains(d.String(), d.Path) {
			t.Errorf("String() = %q", d.String())
		}
	}
	want := map[string]Severity{
		"$schema":                             Warning,
		"basics.name":                         Error,
		"basics.email":                        Warning,
		"basics.url":                          Warning,
		"basics.dateOfBirth":                  Warning,
		"basics.profiles[0].url":              Warning,
		"basics.legalNote.url":                Warning,
		"basics.workAuthorization[0].endDate": Warning,
		"work[0].endDate":                     Warning,
		"work[0].url":                         Warning,
		"work[0].contactDetails.email":        Warning,
		"work[0].positions[0].startDate":      Warning,
		"volunteer[0].endDate":                Warning,
		"volunteer[0].url":                    Warning,
		"education[0].endDate":                Warning,
		"awards[0].date":                      Warning,
		"certificates[0].date":                Warning,
		"certificates[0].url":                 Warning,
		"publications[0].releaseDate":         Warning,
		"publications[0].url":                 Warning,
		"tools[0].url":                        Warning,
		"projects[0].endDate":                 Warning,
		"meta.canonical":                      Warning,
		"meta.lastModified":                   Warning,
		"meta.datePublished":                  Warning,
	}
	for path, sev := range want {
		if got[path] != sev {
			t.Errorf("%s: got %q, want %q", path, got[path], sev)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected diagnostic at %s", path)
		}
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors(nil) || HasErrors([]Diagnostic{{Severity: Warning}}) || !HasErrors([]Diagnostic{{Severity: Warning}, {Severity: Error}}) {
		t.Error("HasErrors")
	}
}

func TestWebURL(t *testing.T) {
	for in, want := range map[string]bool{"https://example.com/x": true, "http://example.com": true,
		"/cv/": false, "javascript:alert(1)": false, "https://": false, "%zz": false} {
		if WebURL(in) != want {
			t.Errorf("WebURL(%q) != %v", in, want)
		}
	}
}
