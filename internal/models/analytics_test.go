package models

import (
	"encoding/json"
	"testing"
)

// TestAnalyticsIDsArrayForm is the regression that started this: wpexporter
// 1.8.x writes one array of ids per vendor, and a build of a real migrated
// site (bociany.pl, 235 posts) died on metadata.json before a single page was
// rendered (#131).
func TestAnalyticsIDsArrayForm(t *testing.T) {
	var meta Metadata
	err := json.Unmarshal([]byte(`{"analytics":{"google_tag_manager":["GTM-WGP8PFC"]}}`), &meta)
	if err != nil {
		t.Fatalf("array form must decode, got: %v", err)
	}
	if got := meta.Analytics["google_tag_manager"]; got != "GTM-WGP8PFC" {
		t.Fatalf("google_tag_manager = %q, want GTM-WGP8PFC", got)
	}
}

func TestAnalyticsIDsDecoding(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{"scalar form", `{"gtm":"GTM-1"}`, map[string]string{"gtm": "GTM-1"}},
		{"array form", `{"ga4":["G-1","G-2"]}`, map[string]string{"ga4": "G-1"}},
		{"first non-empty id wins", `{"ga4":["","  ","G-3"]}`, map[string]string{"ga4": "G-3"}},
		{"numeric id", `{"hotjar_site_id":3216549}`, map[string]string{"hotjar_site_id": "3216549"}},
		{"numeric id in an array", `{"hotjar_site_id":[3216549]}`, map[string]string{"hotjar_site_id": "3216549"}},
		{"ids are trimmed", `{"gtm":"  GTM-4 "}`, map[string]string{"gtm": "GTM-4"}},
		{"empty vendor dropped", `{"gtm":"","ga4":"G-5"}`, map[string]string{"ga4": "G-5"}},
		{"empty array dropped", `{"gtm":[],"ga4":"G-6"}`, map[string]string{"ga4": "G-6"}},
		{"unreadable value dropped, build survives",
			`{"gtm":{"id":"GTM-7"},"meta_pixel":null,"ga4":true,"ua":"UA-8"}`,
			map[string]string{"ua": "UA-8"}},
		{"no vendors", `{}`, map[string]string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ids AnalyticsIDs
			if err := json.Unmarshal([]byte(tc.input), &ids); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.input, err)
			}
			if len(ids) != len(tc.want) {
				t.Fatalf("got %v, want %v", ids, tc.want)
			}
			for vendor, want := range tc.want {
				if ids[vendor] != want {
					t.Errorf("%s = %q, want %q", vendor, ids[vendor], want)
				}
			}
		})
	}
}

// TestAnalyticsIDsMalformed: a block that is not an object is a broken export,
// not a value to salvage — that one stays an error.
func TestAnalyticsIDsMalformed(t *testing.T) {
	for _, input := range []string{`["GTM-1"]`, `"GTM-1"`, `{`} {
		var ids AnalyticsIDs
		if err := json.Unmarshal([]byte(input), &ids); err == nil {
			t.Errorf("%s should not decode as an analytics block", input)
		}
	}
}

// TestAnalyticsIDsAssignableToSiteData guards the seam the generator relies on:
// it copies Metadata.Analytics into SiteData.Analytics, which the templates
// read as .Site.Analytics.
func TestAnalyticsIDsAssignableToSiteData(t *testing.T) {
	var meta Metadata
	if err := json.Unmarshal([]byte(`{"analytics":{"gtm":["GTM-9"]}}`), &meta); err != nil {
		t.Fatal(err)
	}
	site := SiteData{Analytics: meta.Analytics}
	if site.Analytics["gtm"] != "GTM-9" {
		t.Fatalf(".Site.Analytics.gtm = %q, want GTM-9", site.Analytics["gtm"])
	}
}
