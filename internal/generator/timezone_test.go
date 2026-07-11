package generator

// Tests for the timezone feature (I18N-001): site-wide `timezone:` plus
// per-language `language_timezones:` applied to permalink date tokens and the
// Date/Modified template context.

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// utcNewYearHalfHourBefore is 2023-12-31T23:30:00Z — already 2024-01-01 in
// Warsaw (UTC+1), still 2023-12-31 in New York (UTC-5).
var utcNewYearHalfHourBefore = time.Date(2023, 12, 31, 23, 30, 0, 0, time.UTC)

func TestResolveLocationsInvalidZoneWarnsAndSkips(t *testing.T) {
	siteLoc, langLocs := resolveLocations(Config{
		Timezone:          "Mars/Olympus_Mons",
		LanguageTimezones: map[string]string{"pl": "Europe/Warsaw", "xx": "Not/AZone"},
	})
	if siteLoc != nil {
		t.Errorf("invalid site timezone must resolve to nil (no conversion)")
	}
	if _, ok := langLocs["pl"]; !ok {
		t.Errorf("valid language timezone must resolve")
	}
	if _, ok := langLocs["xx"]; ok {
		t.Errorf("invalid language timezone must be skipped")
	}
}

func TestPageDateSiteTimezone(t *testing.T) {
	g := newTestGen(t, "")
	g.siteLoc, g.langLocs = resolveLocations(Config{Timezone: "Europe/Warsaw"})
	got := g.pageDate(models.Page{}, utcNewYearHalfHourBefore)
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 1 {
		t.Errorf("Warsaw date = %v, want 2024-01-01", got)
	}
	// Zero time passes through untouched.
	if !g.pageDate(models.Page{}, time.Time{}).IsZero() {
		t.Errorf("zero date must stay zero")
	}
}

func TestPageDateNoTimezoneIsPassthrough(t *testing.T) {
	g := newTestGen(t, "")
	got := g.pageDate(models.Page{}, utcNewYearHalfHourBefore)
	if !got.Equal(utcNewYearHalfHourBefore) || got.Location() != time.UTC {
		t.Errorf("without timezone config dates must pass through, got %v", got)
	}
}

func TestPageDatePerLanguageOverride(t *testing.T) {
	g := newTestGen(t, "")
	g.siteLoc, g.langLocs = resolveLocations(Config{
		Timezone:          "Europe/Warsaw",
		LanguageTimezones: map[string]string{"en_US": "America/New_York"},
	})
	// en_US page: New York wins over the site zone → still 2023-12-31.
	got := g.pageDate(models.Page{Lang: "en_US"}, utcNewYearHalfHourBefore)
	if got.Year() != 2023 || got.Day() != 31 {
		t.Errorf("en_US date = %v, want 2023-12-31 (per-language override)", got)
	}
	// Other language falls back to the site zone → 2024-01-01.
	got = g.pageDate(models.Page{Lang: "pl_PL"}, utcNewYearHalfHourBefore)
	if got.Year() != 2024 {
		t.Errorf("pl_PL date = %v, want site-zone 2024-01-01", got)
	}
}

func TestExpandPermalinkHonoursTimezone(t *testing.T) {
	g := newTestGen(t, "")
	g.siteLoc, g.langLocs = resolveLocations(Config{Timezone: "Europe/Warsaw"})
	p := models.Page{Slug: "hello", Date: utcNewYearHalfHourBefore}
	if got := g.expandPermalink("/:year/:month/:day/:slug/", p); got != "2024/01/01/hello" {
		t.Errorf("expandPermalink = %q, want 2024/01/01/hello (Warsaw calendar)", got)
	}
	// Without a timezone the UTC calendar is kept (pre-feature behaviour).
	g.siteLoc = nil
	if got := g.expandPermalink("/:year/:month/:day/:slug/", p); got != "2023/12/31/hello" {
		t.Errorf("expandPermalink = %q, want UTC 2023/12/31/hello", got)
	}
}

func TestPageToTemplateDataDatesInZone(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	g.siteLoc, g.langLocs = resolveLocations(Config{Timezone: "Europe/Warsaw"})
	data := g.pageToTemplateData(models.Page{
		Title: "t", Slug: "s",
		Date:     utcNewYearHalfHourBefore,
		Modified: utcNewYearHalfHourBefore,
	}, true)
	for _, key := range []string{"Date", "Modified"} {
		d, ok := data[key].(time.Time)
		if !ok {
			t.Fatalf("%s missing from template context", key)
		}
		if d.Format("2006-01-02") != "2024-01-01" {
			t.Errorf("%s = %v, want Warsaw 2024-01-01", key, d)
		}
	}
}

func TestNewResolvesTimezones(t *testing.T) {
	g, err := New(Config{
		Domain:            "example.com",
		Timezone:          "Europe/Warsaw",
		LanguageTimezones: map[string]string{"en": "America/New_York"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.siteLoc == nil || !strings.Contains(g.siteLoc.String(), "Warsaw") {
		t.Errorf("siteLoc = %v, want Europe/Warsaw", g.siteLoc)
	}
	if len(g.langLocs) != 1 {
		t.Errorf("langLocs = %v, want one entry", g.langLocs)
	}
}
