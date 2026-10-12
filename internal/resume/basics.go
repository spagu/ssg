package resume

// Basics is the person the document describes.
type Basics struct {
	Name              string              `json:"name"`
	Label             string              `json:"label,omitempty"` // job title, e.g. "Lead DevOps Engineer"
	Title             string              `json:"title,omitempty"` // honorific, e.g. "Dr."
	Age               int                 `json:"age,omitempty"`
	DateOfBirth       Date                `json:"dateOfBirth,omitempty"`
	Gender            string              `json:"gender,omitempty"`
	Image             string              `json:"image,omitempty"`
	Email             string              `json:"email,omitempty"`
	Phone             string              `json:"phone,omitempty"`
	URL               string              `json:"url,omitempty"`
	Summary           string              `json:"summary,omitempty"`
	KeyAchievements   []string            `json:"keyAchievements,omitempty"`
	CoreCompetencies  []string            `json:"coreCompetencies,omitempty"`
	Location          *Location           `json:"location,omitempty"`
	Profiles          []Profile           `json:"profiles,omitempty"`
	Nationalities     []Nationality       `json:"nationalities,omitempty"`
	WorkAuthorization []WorkAuthorization `json:"workAuthorization,omitempty"`
	LegalNote         *LegalNote          `json:"legalNote,omitempty"`
}

// Location is a postal address; work locations add a country name.
type Location struct {
	Address       string `json:"address,omitempty"`
	StreetAddress string `json:"streetAddress,omitempty"`
	PostalCode    string `json:"postalCode,omitempty"`
	City          string `json:"city,omitempty"`
	CountryCode   string `json:"countryCode,omitempty"`
	Region        string `json:"region,omitempty"`
	Country       string `json:"country,omitempty"`
}

// Profile is an account on a network (LinkedIn, GitHub, ...).
type Profile struct {
	Network  string `json:"network,omitempty"`
	Username string `json:"username,omitempty"`
	URL      string `json:"url,omitempty"`
}

// Nationality is a citizenship.
type Nationality struct {
	Country string `json:"country,omitempty"`
	Born    bool   `json:"born,omitempty"`
}

// WorkAuthorization is the right to work in one country.
type WorkAuthorization struct {
	Country     string `json:"country,omitempty"`
	Status      string `json:"status,omitempty"`
	RightToWork bool   `json:"rightToWork,omitempty"`
	VisaType    string `json:"visaType,omitempty"`
	ValidFrom   Date   `json:"validFrom,omitempty"`
	ValidTo     Date   `json:"validTo,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// LegalNote is a consent or legal statement, such as a GDPR clause.
type LegalNote struct {
	Text    string `json:"text,omitempty"`
	Country string `json:"country,omitempty"`
	Type    string `json:"type,omitempty"`
	URL     string `json:"url,omitempty"`
}

// ContactDetails are an employer's contact points.
type ContactDetails struct {
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	Fax   string `json:"fax,omitempty"`
}
