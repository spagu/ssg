package generator

// An Agentic Commerce Protocol product feed (GO-161, #197).
//
// The same products as the Google Merchant feed (#196), read by the same
// code — merchantItemsOf over the merged Product + Offer JSON-LD and the
// page's `merchant:` front matter — and written in the field names of the
// OpenAI product feed schema. ACP adds a seller block the feed repeats on
// every row (from the feed's `acp:` config), variant options and a few
// per-product fields a page can give in `acp:` front matter.
//
// The file is JSON Lines: one object per line, the shape of the
// specification's own example. Arrays and the variant map stay JSON rather
// than the comma-joined and JSON-in-a-cell encodings CSV/TSV would need, and
// encoding/json does the escaping.
//
// Only the file is written. Sending it to OpenAI (a hosted URL, an upload or
// SFTP) and the checkout itself are not ssg's: they belong to the shop worker
// and a Cron Trigger beside it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// acpFormat is the `format:` that asks for an ACP product feed.
const acpFormat = "acp"

// acpRow is one line of the feed. Field order is the order written; the
// required fields carry no omitempty.
type acpRow struct {
	ItemID               string            `json:"item_id"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	URL                  string            `json:"url"`
	Brand                string            `json:"brand"`
	ImageURL             string            `json:"image_url"`
	AdditionalImageURLs  []string          `json:"additional_image_urls,omitempty"`
	Price                string            `json:"price"`
	SalePrice            string            `json:"sale_price,omitempty"`
	Availability         string            `json:"availability"`
	Condition            string            `json:"condition,omitempty"`
	GTIN                 string            `json:"gtin,omitempty"`
	MPN                  string            `json:"mpn,omitempty"`
	GroupID              string            `json:"group_id,omitempty"`
	ListingHasVariations bool              `json:"listing_has_variations,omitempty"`
	VariantDict          map[string]string `json:"variant_dict,omitempty"`
	OfferID              string            `json:"offer_id,omitempty"`
	ProductCategory      string            `json:"product_category,omitempty"`
	Material             string            `json:"material,omitempty"`
	Color                string            `json:"color,omitempty"`
	Size                 string            `json:"size,omitempty"`
	SizeSystem           string            `json:"size_system,omitempty"`
	Gender               string            `json:"gender,omitempty"`
	AgeGroup             string            `json:"age_group,omitempty"`
	ReviewCount          int               `json:"review_count,omitempty"`
	StarRating           string            `json:"star_rating,omitempty"`
	IsDigital            *bool             `json:"is_digital,omitempty"`
	ShippingPrice        string            `json:"shipping_price,omitempty"`
	SellerName           string            `json:"seller_name"`
	SellerURL            string            `json:"seller_url,omitempty"`
	SellerPrivacyPolicy  string            `json:"seller_privacy_policy,omitempty"`
	SellerTOS            string            `json:"seller_tos,omitempty"`
	ReturnPolicy         string            `json:"return_policy,omitempty"`
	AcceptsReturns       *bool             `json:"accepts_returns,omitempty"`
	ReturnDeadlineInDays int               `json:"return_deadline_in_days,omitempty"`
	TargetCountries      []string          `json:"target_countries"`
	StoreCountry         string            `json:"store_country"`
	IsEligibleSearch     *bool             `json:"is_eligible_search,omitempty"`
	IsEligibleCheckout   *bool             `json:"is_eligible_checkout,omitempty"`
}

// acpEntry is a row with the product it was read from, for check_acp.
type acpEntry struct {
	row  acpRow
	item merchantItem
}

// acpFeedRecord is one written ACP feed: its path, its config, and every
// row the pages gave — the ones left out too, so the check can name them.
type acpFeedRecord struct {
	rel     string
	spec    models.FeedSpec
	entries []acpEntry
}

// writeACPFeed writes the products of the spec's pages as an ACP feed. A
// row missing a required field is left out, as OpenAI would reject it;
// check_acp names it and the field.
func (g *Generator) writeACPFeed(spec models.FeedSpec, rel string) error {
	if missing := acpSellerMissing(spec.ACP); len(missing) > 0 {
		return fmt.Errorf("feeds: %q (format: acp) needs acp.%s", spec.Path, strings.Join(missing, ", acp."))
	}
	var entries []acpEntry
	for _, p := range g.merchantPages(spec) {
		items := g.merchantItemsOf(p, spec.Merchant)
		for _, it := range items {
			entries = append(entries, acpEntry{row: g.acpRowOf(it, spec.ACP, len(items) == 1), item: it})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].row.ItemID < entries[j].row.ItemID })
	g.acpFeeds = append(g.acpFeeds, acpFeedRecord{rel: rel, spec: spec, entries: entries})

	var rows []acpRow
	unstated := 0
	for _, e := range entries {
		if !e.item.stockFromPage {
			unstated++
		}
		if len(e.row.missingRequired()) == 0 {
			rows = append(rows, e.row)
		}
	}
	if spec.Items != nil && *spec.Items > 0 && len(rows) > *spec.Items {
		rows = rows[:*spec.Items]
	}
	if dropped := len(entries) - len(rows); g.resolveMode(g.config.CheckACP) == "" && (dropped > 0 || unstated > 0) && !g.config.Quiet {
		fmt.Printf("   ⚠️  %s: %d product(s) left out for a missing required field, %d with an availability no page states; set check_acp: warn for the list\n",
			rel, len(entries)-len(rows), unstated)
	}
	return g.writeFeedFile(rel, renderACPFeed(rows), len(rows))
}

// acpSellerMissing names the seller fields the feed requires on every row
// and only the config can give.
func acpSellerMissing(s models.ACPSeller) []string {
	var missing []string
	if strings.TrimSpace(s.SellerName) == "" {
		missing = append(missing, "seller_name")
	}
	if len(s.TargetCountries) == 0 {
		missing = append(missing, "target_countries")
	}
	if strings.TrimSpace(s.StoreCountry) == "" {
		missing = append(missing, "store_country")
	}
	return missing
}

// missingRequired names the per-product fields OpenAI rejects a row without.
// item_id and availability always have a value; the seller fields are the
// config's, checked before any row is built.
func (r acpRow) missingRequired() []string {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"title", r.Title}, {"description", r.Description}, {"url", r.URL},
		{"brand", r.Brand}, {"image_url", r.ImageURL}, {"price", r.Price},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	return missing
}

// renderACPFeed writes one JSON object per line. HTML escaping is off: the
// values are data, not markup, and "&" reads better than "&".
func renderACPFeed(rows []acpRow) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		_ = enc.Encode(r) // strings, ints, bools and a string map cannot fail
	}
	return b.String()
}
