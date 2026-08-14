package models

// Tracking ids as an export records them. The shape is not ssg's to dictate:
// metadata.json is written by an exporter, and wpexporter reports every id it
// found per vendor — so `analytics` arrives as {"google_tag_manager": ["GTM-…"]}
// (>= 1.8.0) or as {"gtm": "GTM-…"} from a hand-written or older export.
//
// Before this type a plain map[string]string met the array form with
//
//	json: cannot unmarshal array into Go struct field Metadata.analytics of type string
//
// and, because metadata.json is loaded before anything else, the whole build
// died: no pages, no posts, no menu, no colours — over an optional block of
// third-party ids that renders only when `analytics: true` (#131).

import (
	"encoding/json"
	"strings"
)

// AnalyticsIDs maps a vendor key ("gtm", "ga4", "meta_pixel", …) to the one
// tracking id the generator will emit for it. It decodes both the scalar and
// the array form; numbers decode too, since ids like a Hotjar site id are
// naturally numeric and JSON keeps them unquoted.
type AnalyticsIDs map[string]string

// UnmarshalJSON reads the export's `analytics` block leniently. A value it
// cannot read as an id is dropped rather than failing the build — a tracking
// id the generator does not understand is not worth a site that does not
// build. Only a malformed block (not an object) is an error.
func (a *AnalyticsIDs) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	ids := make(AnalyticsIDs, len(raw))
	for vendor, value := range raw {
		if id := firstAnalyticsID(value); id != "" {
			ids[vendor] = id
		}
	}
	*a = ids
	return nil
}

// firstAnalyticsID picks the id to use for one vendor. A crawl can find the
// same vendor embedded more than once (a container in the head, another in a
// plugin's footer); the generator emits one snippet per vendor, so the first
// id wins — the exporter reports them in the order the page declared them.
func firstAnalyticsID(value json.RawMessage) string {
	var list []json.RawMessage
	if err := json.Unmarshal(value, &list); err == nil {
		for _, item := range list {
			if id := scalarAnalyticsID(item); id != "" {
				return id
			}
		}
		return ""
	}
	return scalarAnalyticsID(value)
}

// scalarAnalyticsID reads a single id, quoted or not. Anything else — null, an
// object, a bool — is not an id and yields "".
func scalarAnalyticsID(value json.RawMessage) string {
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var number json.Number
	if err := json.Unmarshal(value, &number); err == nil {
		return number.String()
	}
	return ""
}
