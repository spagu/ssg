package generator

// One ACP feed row from one product (GO-161): the Merchant row's values in
// ACP's words, the seller block, and what only ACP asks for — read from the
// Product node first and the page's `acp:` front matter over it.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// acpSharedKeys are `acp:` front-matter keys that describe the whole page,
// so every variant on it takes them. acpSingleKeys name one offer and apply
// only to a page with one product, as `merchant.id` does.
var (
	acpSharedKeys = []string{"product_category", "material", "gender", "age_group", "size_system", "shipping_price"}
	acpSingleKeys = []string{"offer_id", "color", "size", "sale_price"}
)

// acpRowOf builds the row for one product. single says the page has one
// product, which is when the per-offer front-matter keys apply.
func (g *Generator) acpRowOf(it merchantItem, s models.ACPSeller, single bool) acpRow {
	node, offer := it.node, ldMap(firstLD(it.node["offers"]))
	row := acpRow{
		ItemID: it.id, Title: it.title, Description: it.description, URL: it.link,
		Brand: it.brand, ImageURL: it.image, AdditionalImageURLs: g.acpMoreImages(node["image"]),
		Price: it.price, Availability: acpAvailability(it.availability, it.stockUnknown), Condition: it.condition,
		GTIN: it.gtin, MPN: it.mpn, OfferID: ldText(offer["sku"]),
		ProductCategory: ldName(node["category"]), Material: ldName(node["material"]),
		Color: ldName(node["color"]), Size: ldName(node["size"]),
		ShippingPrice: firstNonEmpty(acpShippingRate(offer), strings.TrimSpace(s.ShippingPrice)),
		IsDigital:     s.IsDigital,
		SellerName:    strings.TrimSpace(s.SellerName), SellerURL: g.socialURL(s.SellerURL),
		SellerPrivacyPolicy: g.socialURL(s.SellerPrivacyPolicy), SellerTOS: g.socialURL(s.SellerTOS),
		ReturnPolicy: g.socialURL(s.ReturnPolicy), AcceptsReturns: s.AcceptsReturns,
		TargetCountries: upperAll(s.TargetCountries), StoreCountry: strings.ToUpper(strings.TrimSpace(s.StoreCountry)),
		IsEligibleSearch: s.IsEligibleSearch, IsEligibleCheckout: s.IsEligibleCheckout,
	}
	if s.AcceptsReturns != nil && *s.AcceptsReturns && s.ReturnDeadlineInDays > 0 {
		row.ReturnDeadlineInDays = s.ReturnDeadlineInDays
	}
	row.ReviewCount, row.StarRating = acpRating(ldMap(node["aggregateRating"]))
	if it.group != nil {
		row.GroupID, row.ListingHasVariations = it.groupID, true
		row.VariantDict = acpVariantDict(node, it.group)
	}
	row.applyFront(ldMap(it.front["acp"]), single)
	return row
}

// applyFront lays the page's `acp:` keys over the row.
func (r *acpRow) applyFront(front map[string]interface{}, single bool) {
	fields := map[string]*string{
		"product_category": &r.ProductCategory, "material": &r.Material, "gender": &r.Gender,
		"age_group": &r.AgeGroup, "size_system": &r.SizeSystem, "shipping_price": &r.ShippingPrice,
		"offer_id": &r.OfferID, "color": &r.Color, "size": &r.Size, "sale_price": &r.SalePrice,
	}
	keys := acpSharedKeys
	if single {
		keys = append(append([]string(nil), acpSharedKeys...), acpSingleKeys...)
	}
	for _, key := range keys {
		if v := ldText(front[key]); v != "" {
			*fields[key] = v
		}
	}
	for key, field := range map[string]**bool{
		"is_digital": &r.IsDigital, "is_eligible_search": &r.IsEligibleSearch, "is_eligible_checkout": &r.IsEligibleCheckout,
	} {
		if v, ok := front[key].(bool); ok {
			*field = &v
		}
	}
}

// acpAvailability is the Merchant value in ACP's spelling: the two differ
// on pre-orders, and on `unknown`, which OpenAI's product feed accepts ("Use
// unknown explicitly when stock status is unavailable", read 2026-11-15) and
// Google's does not, so the Merchant value of a page saying `unknown` is
// in_stock (GO-187).
func acpAvailability(merchant string, unknown bool) string {
	switch {
	case unknown:
		return "unknown"
	case merchant == "preorder":
		return "pre_order"
	}
	return merchant
}

// acpMoreImages are the product's images after the first, absolute.
func (g *Generator) acpMoreImages(v interface{}) []string {
	var out []string
	for i, img := range ldList(v) {
		if u := g.socialURL(ldImage(img)); i > 0 && u != "" {
			out = append(out, u)
		}
	}
	return out
}

// acpShippingRate is an offer's shippingDetails.shippingRate as "5.00 EUR".
func acpShippingRate(offer map[string]interface{}) string {
	rate := ldMap(firstLD(ldMap(firstLD(offer["shippingDetails"]))["shippingRate"]))
	value, currency := ldText(rate["value"]), ldText(rate["currency"])
	if value == "" || currency == "" {
		return ""
	}
	return value + " " + strings.ToUpper(currency)
}

// acpRating reads an AggregateRating: the count, and the rating on the
// feed's 0–5 scale with two decimals. No count, no rating — the feed pairs
// them.
func acpRating(r map[string]interface{}) (int, string) {
	count, _ := strconv.Atoi(firstNonEmpty(ldText(r["reviewCount"]), ldText(r["ratingCount"])))
	value, err := strconv.ParseFloat(ldText(r["ratingValue"]), 64)
	if count <= 0 || err != nil {
		return 0, ""
	}
	if best, e := strconv.ParseFloat(ldText(r["bestRating"]), 64); e == nil && best > 0 {
		value = value * 5 / best
	}
	return count, fmt.Sprintf("%.2f", value)
}

// acpVariantDict maps a variant's options to their values: the properties
// its group says it varies by (variesBy), else its color and size.
func acpVariantDict(variant, group map[string]interface{}) map[string]string {
	opts := variantOptions(variant, group)
	if len(opts) == 0 {
		return nil
	}
	out := make(map[string]string, len(opts))
	for _, o := range opts {
		out[o.name] = o.value
	}
	return out
}

// upperAll is a list of codes in upper case, blanks dropped.
func upperAll(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}
