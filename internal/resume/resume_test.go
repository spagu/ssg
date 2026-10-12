package resume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleDoc is a document touching every section, with a custom field and
// dates at all three precisions.
const sampleDoc = `{
  "$schema": "https://schema-resume.org/1.3/schema.json",
  "@context": "https://schema-resume.org/schema.json",
  "basics": {
    "name": "Ada Example", "label": "Engineer", "title": "Dr.",
    "email": "ada@example.com", "phone": "+1 555 0100", "url": "https://example.com",
    "image": "https://example.com/ada.jpg", "summary": "Builds things.",
    "location": {"streetAddress": "1 Main St\nFlat 2", "postalCode": "00-001", "city": "Springfield", "region": "North", "countryCode": "EX"},
    "profiles": [{"network": "Code", "username": "ada", "url": "https://code.example.com/ada"}],
    "keyAchievements": ["Shipped it"], "coreCompetencies": ["Ops"],
    "pronouns": "she/her"
  },
  "work": [
    {"name": "Acme", "url": "https://acme.example.com", "position": "Lead", "startDate": "2021-06", "highlights": ["Led"]},
    {"name": "Initech", "positions": [
      {"position": "Dev", "startDate": "2015", "endDate": "2017-12"},
      {"position": "Senior Dev", "startDate": "2018-01-15", "endDate": "2020-12-31"}
    ]}
  ],
  "education": [{"institution": "Example University", "area": "CS", "studyType": "BSc", "startDate": "2011", "endDate": "2015"}],
  "skills": [{"name": "Go", "level": "Expert", "yearsOfExperience": 7.5, "keywords": ["Concurrency"]}],
  "meta": {"version": "v1", "dateModified": "2026-10-01T12:00:00Z"},
  "favouriteColour": "green"
}`

func TestParseKeepsCustomFieldsAndPartialDates(t *testing.T) {
	r, err := Parse([]byte(sampleDoc))
	if err != nil {
		t.Fatal(err)
	}
	if r.Basics.Name != "Ada Example" || r.Basics.Location.City != "Springfield" || len(r.Work) != 2 {
		t.Fatalf("decoded %+v", r.Basics)
	}
	if r.Raw["favouriteColour"] != "green" || r.Raw["basics"].(map[string]interface{})["pronouns"] != "she/her" {
		t.Error("custom fields must stay in Raw")
	}
	if string(r.Source) != sampleDoc {
		t.Error("Source must be the bytes as read")
	}
	if r.Skills[0].YearsOfExperience != 7.5 || r.Work[1].Positions[1].StartDate != "2018-01-15" {
		t.Error("numbers and full dates decode")
	}
	if diags := Validate(r); len(diags) != 0 {
		t.Errorf("a clean document has no diagnostics: %v", diags)
	}
}

func TestParseRejectsNonObjects(t *testing.T) {
	for _, in := range []string{`[]`, `{"basics": "x"}`, `not json`} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s must not parse", in)
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cv.json")
	if err := os.WriteFile(path, []byte(`{"basics": {}, "work": [{"startDate": "2020-13"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r, diags, err := Load(path)
	if err != nil || r == nil {
		t.Fatal(err)
	}
	if !HasErrors(diags) || len(diags) != 2 {
		t.Errorf("diags = %v", diags)
	}
	if _, _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file is an error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`[`), 0o600)
	if _, _, err := Load(bad); err == nil {
		t.Error("broken JSON is an error")
	}
}

func TestDate(t *testing.T) {
	for in, want := range map[Date]bool{"2019": true, "2019-06": true, "2019-06-15": true,
		"2019-13": false, "2019-02-30": false, "19": false, "2019/06": false, "": false, "3000": false} {
		if got := in.Valid(); got != want {
			t.Errorf("%q.Valid() = %v", in, got)
		}
	}
	if tm, ok := Date("2019-06").Time(); !ok || tm.Month() != 6 || tm.Day() != 1 {
		t.Errorf("Time = %v", tm)
	}
	if !Date("2019").Before("2020-01") || Date("2020").Before("2020-06") || Date("2020-07").Before("2020-06") {
		t.Error("Before compares at the coarser precision")
	}
}

func TestWorkCurrentAndEducationOrganization(t *testing.T) {
	cases := []struct {
		w    Work
		want bool
	}{
		{Work{StartDate: "2020"}, true},
		{Work{StartDate: "2020", EndDate: "2021"}, false},
		{Work{Positions: []Position{{EndDate: "2019"}, {StartDate: "2019"}}}, true},
		{Work{Positions: []Position{{EndDate: "2019"}}}, false},
		{Work{EndDate: "2019", Positions: []Position{{StartDate: "2019"}}}, false},
	}
	for i, c := range cases {
		if got := c.w.Current(); got != c.want {
			t.Errorf("case %d: Current() = %v", i, got)
		}
	}
	if (Education{School: "S"}).Organization() != "S" || (Education{Institution: "I", School: "S"}).Organization() != "I" {
		t.Error("Organization prefers the institution")
	}
}

func TestWithoutContact(t *testing.T) {
	r, _ := Parse([]byte(sampleDoc))
	clean, err := r.WithoutContact()
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"ada@example.com", "555 0100", "Main St", "00-001"} {
		if strings.Contains(string(clean.Source), leak) {
			t.Errorf("%q must not be published", leak)
		}
	}
	if clean.Basics.Email != "" || clean.Basics.Phone != "" || clean.Basics.Location.StreetAddress != "" {
		t.Error("typed fields are redacted too")
	}
	if clean.Basics.Location.City != "Springfield" || clean.Raw["favouriteColour"] != "green" || !strings.Contains(string(clean.Source), "she/her") {
		t.Error("everything else stays, custom fields included")
	}
	if r.Basics.Email == "" || !strings.Contains(string(r.Source), "ada@example.com") {
		t.Error("the receiver is not changed")
	}
	plain, _ := Parse([]byte(`{"basics": {"name": "N", "location": {"city": "C"}}}`))
	if same, err := plain.WithoutContact(); err != nil || same != plain {
		t.Error("nothing to take out keeps the document as written")
	}
	empty, _ := Parse([]byte(`null`))
	if _, err := empty.WithoutContact(); err != nil {
		t.Error(err)
	}
}

func TestWithoutContactEncodeError(t *testing.T) {
	r := &Resume{Raw: map[string]interface{}{"basics": map[string]interface{}{"email": "x"}, "bad": func() {}}}
	if _, err := r.WithoutContact(); err == nil {
		t.Error("an unencodable value is an error")
	}
}
