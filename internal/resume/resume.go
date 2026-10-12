// Package resume reads a Schema Resume document (https://schema-resume.org):
// a CV as JSON, with sections for the person (basics), their work, education,
// skills and the rest. The types follow schema 1.3; any field they do not
// name is kept in Resume.Raw, so a custom field still reaches a theme.
//
// Nothing here touches the network: the document is read and checked
// against the rules the schema states (a name, ISO 8601 dates, absolute
// URLs), and problems come back as diagnostics.
package resume

// Resume is one Schema Resume document.
type Resume struct {
	Schema       string        `json:"$schema,omitempty"`
	Context      interface{}   `json:"@context,omitempty"` // a URL, or a JSON-LD context object/array
	Basics       Basics        `json:"basics"`
	Work         []Work        `json:"work,omitempty"`
	Volunteer    []Volunteer   `json:"volunteer,omitempty"`
	Education    []Education   `json:"education,omitempty"`
	Awards       []Award       `json:"awards,omitempty"`
	Certificates []Certificate `json:"certificates,omitempty"`
	Publications []Publication `json:"publications,omitempty"`
	Skills       []Skill       `json:"skills,omitempty"`
	Tools        []Tool        `json:"tools,omitempty"`
	Languages    []Language    `json:"languages,omitempty"`
	Interests    []Interest    `json:"interests,omitempty"`
	References   []Reference   `json:"references,omitempty"`
	Projects     []Project     `json:"projects,omitempty"`
	Meta         Meta          `json:"meta"`

	// Raw is the whole document as decoded JSON, custom fields included.
	Raw map[string]interface{} `json:"-"`
	// Source is the document's bytes, as read (or as redacted).
	Source []byte `json:"-"`
}

// Work is one employer; Positions lists several roles held there.
type Work struct {
	Name           string          `json:"name,omitempty"`
	Industry       string          `json:"industry,omitempty"`
	Location       *Location       `json:"location,omitempty"`
	WorkLocation   string          `json:"workLocation,omitempty"`
	ContactDetails *ContactDetails `json:"contactDetails,omitempty"`
	Description    string          `json:"description,omitempty"`
	Position       string          `json:"position,omitempty"`
	WorkType       string          `json:"workType,omitempty"`
	URL            string          `json:"url,omitempty"`
	StartDate      Date            `json:"startDate,omitempty"`
	EndDate        Date            `json:"endDate,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	Highlights     []string        `json:"highlights,omitempty"`
	Positions      []Position      `json:"positions,omitempty"`
}

// Position is one role within a Work entry.
type Position struct {
	Position   string   `json:"position,omitempty"`
	WorkType   string   `json:"workType,omitempty"`
	StartDate  Date     `json:"startDate,omitempty"`
	EndDate    Date     `json:"endDate,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
}

// Current reports whether the person still works there: no end date, or —
// for an entry that dates only its positions — an open position.
func (w Work) Current() bool {
	if len(w.Positions) > 0 && w.StartDate == "" && w.EndDate == "" {
		for _, p := range w.Positions {
			if p.EndDate == "" {
				return true
			}
		}
		return false
	}
	return w.EndDate == ""
}

// Volunteer is unpaid work.
type Volunteer struct {
	Organization string   `json:"organization,omitempty"`
	Position     string   `json:"position,omitempty"`
	URL          string   `json:"url,omitempty"`
	StartDate    Date     `json:"startDate,omitempty"`
	EndDate      Date     `json:"endDate,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Highlights   []string `json:"highlights,omitempty"`
}

// Education is a course of study.
type Education struct {
	Institution string   `json:"institution,omitempty"`
	School      string   `json:"school,omitempty"`
	URL         string   `json:"url,omitempty"`
	Area        string   `json:"area,omitempty"`
	StudyType   string   `json:"studyType,omitempty"`
	Degree      string   `json:"degree,omitempty"`
	Description string   `json:"description,omitempty"`
	StartDate   Date     `json:"startDate,omitempty"`
	EndDate     Date     `json:"endDate,omitempty"`
	Score       string   `json:"score,omitempty"`
	GPA         string   `json:"gpa,omitempty"`
	Location    string   `json:"location,omitempty"`
	Country     string   `json:"country,omitempty"`
	Courses     []string `json:"courses,omitempty"`
}

// Organization is the institution's name, falling back to the school's.
func (e Education) Organization() string {
	if e.Institution != "" {
		return e.Institution
	}
	return e.School
}

// Award is a prize or recognition.
type Award struct {
	Title   string `json:"title,omitempty"`
	Date    Date   `json:"date,omitempty"`
	Awarder string `json:"awarder,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// Certificate is a professional credential.
type Certificate struct {
	Name   string `json:"name,omitempty"`
	Date   Date   `json:"date,omitempty"`
	URL    string `json:"url,omitempty"`
	Issuer string `json:"issuer,omitempty"`
}

// Publication is an article, paper or book.
type Publication struct {
	Name        string `json:"name,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	ReleaseDate Date   `json:"releaseDate,omitempty"`
	URL         string `json:"url,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

// Skill is an area of competence with its keywords.
type Skill struct {
	Name              string   `json:"name,omitempty"`
	Level             string   `json:"level,omitempty"`
	YearsOfExperience float64  `json:"yearsOfExperience,omitempty"`
	Comment           string   `json:"comment,omitempty"`
	Keywords          []string `json:"keywords,omitempty"`
}

// Tool is a piece of software the person uses.
type Tool struct {
	Name              string  `json:"name,omitempty"`
	Group             string  `json:"group,omitempty"`
	Category          string  `json:"category,omitempty"`
	YearsOfExperience float64 `json:"yearsOfExperience,omitempty"`
	Level             string  `json:"level,omitempty"`
	Comment           string  `json:"comment,omitempty"`
	URL               string  `json:"url,omitempty"`
}

// Language is a spoken language and how well.
type Language struct {
	Language string `json:"language,omitempty"`
	Fluency  string `json:"fluency,omitempty"`
}

// Interest is a pastime or field of interest.
type Interest struct {
	Name     string   `json:"name,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

// Reference is a recommendation from someone.
type Reference struct {
	Name      string `json:"name,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// Project is a piece of work outside (or alongside) employment.
type Project struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Highlights  []string `json:"highlights,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	StartDate   Date     `json:"startDate,omitempty"`
	EndDate     Date     `json:"endDate,omitempty"`
	URL         string   `json:"url,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Entity      string   `json:"entity,omitempty"`
	Type        string   `json:"type,omitempty"`
}

// Meta describes the document itself.
type Meta struct {
	Canonical     string `json:"canonical,omitempty"`
	Version       string `json:"version,omitempty"`
	LastModified  string `json:"lastModified,omitempty"`
	DateCreated   string `json:"dateCreated,omitempty"`
	DateModified  string `json:"dateModified,omitempty"`
	DatePublished string `json:"datePublished,omitempty"`
}
