package generator

// The Agentic Commerce Protocol product feed (GO-161).

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// acpSeller is a complete seller block.
func acpSeller() models.ACPSeller {
	yes, days := true, 30
	return models.ACPSeller{SellerName: "Example Shop", SellerURL: "/", SellerPrivacyPolicy: "/privacy/",
		SellerTOS: "https://example.com/terms/", ReturnPolicy: "/returns/", AcceptsReturns: &yes,
		ReturnDeadlineInDays: days, TargetCountries: []string{" us", ""}, StoreCountry: "us"}
}

// writeACP writes an acp feed and returns its rows by item_id, and the file.
func writeACP(t *testing.T, g *Generator, out string, spec models.FeedSpec) (map[string]map[string]interface{}, string) {
	t.Helper()
	spec.Format = acpFormat
	if spec.ACP.SellerName == "" {
		spec.ACP = acpSeller()
	}
	if err := g.writeDeclaredFeed(spec); err != nil {
		t.Fatal(err)
	}
	body := mustRead(t, filepath.Join(out, strings.TrimPrefix(spec.Path, "/")))
	rows := map[string]map[string]interface{}{}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if line == "" {
			continue
		}
		var row map[string]interface{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("not one JSON object per line: %v\n%s", err, line)
		}
		rows[row["item_id"].(string)] = row
	}
	return rows, body
}

var acpLamp = map[string]interface{}{
	"@type": "Product", "name": "Lamp & Shade", "sku": "LAMP-1", "category": "Home > Lighting",
	"material": "Brass", "color": "Gold", "size": map[string]interface{}{"name": "M"},
	"image":           []interface{}{"/lamp.jpg", "/lamp-side.jpg", map[string]interface{}{"url": "/lamp-top.jpg"}},
	"aggregateRating": map[string]interface{}{"ratingValue": "9", "bestRating": 10, "reviewCount": 4},
	"offers": map[string]interface{}{"price": "40.00", "priceCurrency": "eur", "sku": "OFFER-LAMP",
		"availability":    "https://schema.org/PreOrder",
		"shippingDetails": map[string]interface{}{"shippingRate": map[string]interface{}{"value": "4.90", "currency": "eur"}}},
}

// TestACPFeedRowFields: every field of a plain product, one assertion each.
func TestACPFeedRowFields(t *testing.T) {
	g, out := merchantGen(t, []models.Page{productPage("lamp", acpLamp, map[string]interface{}{
		"merchant": map[string]interface{}{"brand": "Lumen", "condition": "used"},
		"acp": map[string]interface{}{"gender": "unisex", "age_group": "adult", "sale_price": "35.00 EUR",
			"is_digital": false, "is_eligible_checkout": true, "offer_id": "LAMP-OFFER-7"},
	})}, nil)
	rows, body := writeACP(t, g, out, models.FeedSpec{Path: "/acp.jsonl"})
	lamp := rows["LAMP-1"]
	for _, tc := range []struct {
		field string
		want  interface{}
	}{
		{"title", "Lamp & Shade"}, {"description", "About lamp"}, {"url", "https://example.com/lamp/"},
		{"brand", "Lumen"}, {"image_url", "https://example.com/lamp.jpg"}, {"price", "40.00 EUR"},
		{"sale_price", "35.00 EUR"}, {"availability", "pre_order"}, {"condition", "used"},
		{"offer_id", "LAMP-OFFER-7"}, {"product_category", "Home > Lighting"}, {"material", "Brass"},
		{"color", "Gold"}, {"size", "M"}, {"gender", "unisex"}, {"age_group", "adult"},
		{"review_count", 4.0}, {"star_rating", "4.50"}, {"is_digital", false}, {"shipping_price", "4.90 EUR"},
		{"seller_name", "Example Shop"}, {"seller_url", "https://example.com/"},
		{"seller_privacy_policy", "https://example.com/privacy/"}, {"seller_tos", "https://example.com/terms/"},
		{"return_policy", "https://example.com/returns/"}, {"accepts_returns", true},
		{"return_deadline_in_days", 30.0}, {"store_country", "US"}, {"is_eligible_checkout", true},
	} {
		if got := lamp[tc.field]; got != tc.want {
			t.Errorf("%s = %#v, want %#v", tc.field, got, tc.want)
		}
	}
	if imgs := lamp["additional_image_urls"].([]interface{}); len(imgs) != 2 || imgs[1] != "https://example.com/lamp-top.jpg" {
		t.Errorf("additional_image_urls = %v", imgs)
	}
	if c := lamp["target_countries"].([]interface{}); len(c) != 1 || c[0] != "US" {
		t.Errorf("target_countries = %v", c)
	}
	for _, absent := range []string{"group_id", "variant_dict", "listing_has_variations", "is_eligible_search", "gtin"} {
		if _, ok := lamp[absent]; ok {
			t.Errorf("%s written for a product without it", absent)
		}
	}
	if !strings.Contains(body, `"Lamp & Shade"`) {
		t.Error("values are data: no HTML escaping")
	}
}

// TestACPFeedVariants: a ProductGroup is one row per variant with group_id,
// listing_has_variations and variant_dict from variesBy; per-offer front
// matter stays off variants, page-wide front matter reaches them all.
func TestACPFeedVariants(t *testing.T) {
	group := map[string]interface{}{
		"@type": "ProductGroup", "name": "Shirt", "productGroupID": "SHIRT", "image": "/shirt.jpg", "brand": "Wear",
		"variesBy": []interface{}{"https://schema.org/size", "https://schema.org/color"},
		"offers":   map[string]interface{}{"price": "20", "priceCurrency": "EUR", "availability": "InStock"},
		"hasVariant": []interface{}{
			map[string]interface{}{"@type": "Product", "sku": "SHIRT-S", "size": "S", "color": "Red"},
			map[string]interface{}{"@type": "Product", "sku": "SHIRT-M", "size": "M"},
		},
	}
	front := map[string]interface{}{"acp": map[string]interface{}{"offer_id": "ONE", "color": "Blue", "size_system": "EU", "is_eligible_search": false}}
	g, out := merchantGen(t, []models.Page{productPage("shirt", group, front)}, nil)
	rows, _ := writeACP(t, g, out, models.FeedSpec{Path: "/acp.jsonl"})
	s, m := rows["SHIRT-S"], rows["SHIRT-M"]
	if s["group_id"] != "SHIRT" || s["listing_has_variations"] != true || m["group_id"] != "SHIRT" {
		t.Errorf("variant rows: %v %v", s, m)
	}
	if d := s["variant_dict"].(map[string]interface{}); d["size"] != "S" || d["color"] != "Red" {
		t.Errorf("variant_dict = %v", d)
	}
	if d := m["variant_dict"].(map[string]interface{}); len(d) != 1 || d["size"] != "M" {
		t.Errorf("an option a variant lacks is left out: %v", d)
	}
	if s["offer_id"] != nil || s["color"] != "Red" || s["size_system"] != "EU" || m["is_eligible_search"] != false {
		t.Errorf("front matter on variants: %v", s)
	}
}

// TestACPFeedSellerRequired: the config's required seller fields fail the
// build by name rather than writing rows OpenAI rejects.
func TestACPFeedSellerRequired(t *testing.T) {
	g, _ := merchantGen(t, nil, nil)
	err := g.writeDeclaredFeed(models.FeedSpec{Path: "/acp.jsonl", Format: "ACP"})
	if err == nil || !strings.Contains(err.Error(), "acp.seller_name, acp.target_countries, acp.store_country") {
		t.Errorf("err = %v", err)
	}
	if _, _, err := feedFormatOf(models.FeedSpec{Format: "csv"}); err == nil || !strings.Contains(err.Error(), "acp") {
		t.Errorf("the supported list names acp: %v", err)
	}
}

// TestACPFeedLeavesOutAndCaps: a row missing a required field is not
// written; items: caps what is; the feed is not advertised to readers.
func TestACPFeedLeavesOutAndCaps(t *testing.T) {
	ghost := map[string]interface{}{"@type": "Product", "name": "Ghost", "sku": "GHOST", "offers": map[string]interface{}{"price": 1}}
	g, out := merchantGen(t, []models.Page{productPage("mug", mug, nil), productPage("lamp", acpLamp, nil), productPage("ghost", ghost, nil)}, nil)
	g.config.Quiet = false // the summary line names what check_acp would list
	house := models.MerchantDefaults{Brand: "House"}
	rows, _ := writeACP(t, g, out, models.FeedSpec{Path: "/acp.jsonl", Merchant: house})
	if _, ok := rows["GHOST"]; ok || len(rows) != 2 || rows["LAMP-1"]["brand"] != "House" {
		t.Errorf("rows = %v", rows)
	}
	if rows["MUG-1"]["brand"] != "Example" || rows["MUG-1"]["availability"] != "out_of_stock" {
		t.Errorf("mug row: %v", rows["MUG-1"])
	}
	one := 1
	capped, _ := writeACP(t, g, out, models.FeedSpec{Path: "/one.jsonl", Items: &one, Merchant: house})
	if len(capped) != 1 || capped["LAMP-1"] == nil {
		t.Errorf("items: 1 keeps the first by item_id: %v", capped)
	}
	if len(g.acpFeeds) != 2 || len(g.acpFeeds[0].entries) != 3 {
		t.Errorf("every row is kept for the check: %+v", g.acpFeeds)
	}
	g.config.Feeds = []models.FeedSpec{{Path: "/acp.jsonl", Format: acpFormat}, {Path: "/blog.xml"}}
	if links := g.feedAutodiscoveryLinks(""); strings.Contains(links, "acp.jsonl") {
		t.Errorf("an ACP feed is not for readers: %s", links)
	}
}

// TestACPFeedDeterministic: the same products in any page order give the
// same bytes.
func TestACPFeedDeterministic(t *testing.T) {
	pages := []models.Page{productPage("mug", mug, nil), productPage("lamp", acpLamp, nil)}
	g, out := merchantGen(t, pages, nil)
	_, first := writeACP(t, g, out, models.FeedSpec{Path: "/acp.jsonl"})
	g.siteData.Pages = []models.Page{pages[1], pages[0]}
	for i := 0; i < 5; i++ {
		if _, again := writeACP(t, g, out, models.FeedSpec{Path: "/acp.jsonl"}); again != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first, again)
		}
	}
}

func TestACPHelpers(t *testing.T) {
	for _, tc := range []struct {
		rating      map[string]interface{}
		count       int
		stars, name string
	}{
		{map[string]interface{}{"ratingValue": 4.5, "ratingCount": "12"}, 12, "4.50", "five-point scale"},
		{map[string]interface{}{"ratingValue": "80", "bestRating": "100", "reviewCount": 2}, 2, "4.00", "rescaled"},
		{map[string]interface{}{"ratingValue": "4"}, 0, "", "no count, no rating"},
		{map[string]interface{}{"ratingValue": "x", "reviewCount": 3}, 0, "", "unreadable value"},
		{nil, 0, "", "none"},
	} {
		if c, s := acpRating(tc.rating); c != tc.count || s != tc.stars {
			t.Errorf("%s: %d %q", tc.name, c, s)
		}
	}
	for in, want := range map[string]string{"preorder": "pre_order", "in_stock": "in_stock", "backorder": "backorder"} {
		if got := acpAvailability(in); got != want {
			t.Errorf("availability %q = %q", in, got)
		}
	}
	if merchantAvailability("pre_order") != "preorder" {
		t.Error("ACP's spelling in merchant.availability is a pre-order")
	}
	if acpShippingRate(map[string]interface{}{"shippingDetails": map[string]interface{}{"shippingRate": map[string]interface{}{"value": 5}}}) != "" {
		t.Error("a rate without a currency is no shipping_price")
	}
	if acpVariantDict(map[string]interface{}{}, map[string]interface{}{}) != nil || upperAll(nil) != nil {
		t.Error("empty in, nothing out")
	}
}
