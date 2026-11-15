package generator

// check_acp — the rows of an ACP feed most likely to drift from the checkout
// (GO-162, #197).
//
// What an agent shows comes from the feed; what it charges comes from the
// checkout. When the two disagree the product quietly stops being shown: no
// build failure, no 4xx, nothing in a log. The build is the one point in that
// loop where a person is looking, so this names, per source file and field,
// the rows OpenAI would reject or that state something no page does. Same
// shape as check_schema: "" (off), "warn" or "strict".

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// acpMoneyRe is the feed's money format: a decimal amount with a point, one
// space, an ISO 4217 code — "79.99 USD".
var acpMoneyRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)? [A-Z]{3}$`)

// checkACPIfRequested reports the problems in every ACP feed this build wrote.
func (g *Generator) checkACPIfRequested() error {
	mode := g.resolveMode(g.config.CheckACP)
	if mode == "" || len(g.acpFeeds) == 0 {
		return nil
	}
	g.log("🛒 Checking the ACP product feed...")
	var findings []finding
	for _, rec := range g.acpFeeds {
		findings = append(findings, g.acpFindings(rec)...)
	}
	sortFindings(findings)
	return g.report(findings, mode, "ACP feed",
		"ACP feed rows carry every required field and agree with the pages",
		"%d ACP feed problem(s)")
}

// acpFindings checks one feed: each row, then the seller block.
func (g *Generator) acpFindings(rec acpFeedRecord) []finding {
	var out []finding
	itemIDs, offerIDs := map[string]string{}, map[string]string{}
	checkout := false
	for _, e := range rec.entries {
		where := firstNonEmpty(e.item.source, e.item.page)
		add := func(field, msg string) {
			out = append(out, finding{where, fmt.Sprintf("%s item %s: %s — %s", rec.rel, e.row.ItemID, field, msg)})
		}
		g.acpRowFindings(e, add)
		for _, dup := range []struct {
			field, value string
			seen         map[string]string
		}{{"item_id", e.row.ItemID, itemIDs}, {"offer_id", e.row.OfferID, offerIDs}} {
			if dup.value == "" {
				continue
			}
			if first, ok := dup.seen[dup.value]; ok {
				add(dup.field, "also used by "+first+"; it must be unique in the feed")
				continue
			}
			dup.seen[dup.value] = where
		}
		checkout = checkout || acpCheckout(e.row)
	}
	return append(out, g.acpSellerFindings(rec, checkout)...)
}

// acpRowFindings checks one row's own fields.
func (g *Generator) acpRowFindings(e acpEntry, add func(field, msg string)) {
	r := e.row
	if e.item.bareprice != "" {
		add("price", e.item.bareprice+" has no currency; set offers.priceCurrency — the row is left out")
	}
	for _, f := range r.missingRequired() {
		if f != "price" || e.item.bareprice == "" {
			add(f, "required and missing — the row is left out")
		}
	}
	if !e.item.stockFromPage {
		add("availability", r.Availability+" is stated by no page; set offers.availability or merchant.availability")
	}
	if r.URL != "" && !g.acpURLResolves(r.URL) {
		add("url", r.URL+" does not resolve in the output")
	}
	for _, m := range []struct{ field, value string }{
		{"price", r.Price}, {"sale_price", r.SalePrice}, {"shipping_price", r.ShippingPrice},
	} {
		if m.value != "" {
			if msg := acpMoneyProblem(m.value, r.Price, m.field == "sale_price"); msg != "" {
				add(m.field, msg)
			}
		}
	}
	if acpCheckout(r) && r.IsEligibleSearch != nil && !*r.IsEligibleSearch {
		add("is_eligible_checkout", "true while is_eligible_search is false, which turns checkout off")
	}
}

// acpMoneyProblem says what is wrong with an amount, or "". Every amount
// shares the price's currency; a sale price is below the price.
func acpMoneyProblem(value, price string, sale bool) string {
	if !acpMoneyRe.MatchString(value) {
		return value + ` is not "amount CURRENCY", e.g. 79.99 USD`
	}
	if !acpMoneyRe.MatchString(price) {
		return ""
	}
	amount, currency, _ := strings.Cut(value, " ")
	base, baseCurrency, _ := strings.Cut(price, " ")
	if currency != baseCurrency {
		return value + " is not in the price's currency, " + baseCurrency
	}
	a, _ := strconv.ParseFloat(amount, 64)
	b, _ := strconv.ParseFloat(base, 64)
	if sale && (a <= 0 || a >= b) {
		return value + " must be above zero and below the price, " + price
	}
	return ""
}

// acpSellerFindings checks the feed's seller block: the policies checkout
// needs, and that every seller URL on this site resolves.
func (g *Generator) acpSellerFindings(rec acpFeedRecord, checkout bool) []finding {
	var out []finding
	where := "feeds: " + rec.spec.Path
	s := rec.spec.ACP
	for _, f := range []struct{ field, value string }{
		{"seller_url", s.SellerURL}, {"seller_privacy_policy", s.SellerPrivacyPolicy},
		{"seller_tos", s.SellerTOS}, {"return_policy", s.ReturnPolicy},
	} {
		switch u := g.socialURL(f.value); {
		case u == "" && checkout && (f.field == "seller_privacy_policy" || f.field == "seller_tos"):
			out = append(out, finding{where, fmt.Sprintf("acp.%s — required when a row has is_eligible_checkout: true", f.field)})
		case u != "" && !g.acpURLResolves(u):
			out = append(out, finding{where, fmt.Sprintf("acp.%s — %s does not resolve in the output", f.field, u)})
		}
	}
	return out
}

// acpCheckout reports whether a row asks for checkout.
func acpCheckout(r acpRow) bool {
	return r.IsEligibleCheckout != nil && *r.IsEligibleCheckout
}

// acpURLResolves reports whether a URL on this site names a file in the
// output. Another site's URL cannot be checked from here and passes.
func (g *Generator) acpURLResolves(u string) bool {
	ref, ok := g.siteRelativeRef(u)
	if !ok {
		return false
	}
	if !strings.HasPrefix(ref, "/") {
		return true
	}
	return g.refResolves(ref, g.config.OutputDir)
}
