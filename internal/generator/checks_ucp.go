package generator

// check_ucp — the Merchant feed rows that would not get UCP checkout on
// Google (GO-166).
//
// Google drops a product from checkout without telling the build: a missing
// attribute, a restricted kind of product, a notice it cannot show. This
// names each one by source file and field, from the requirements in Google's
// UCP guide (read 2026-11-14). Same shape as check_acp: "" (off), "warn" or
// "strict". What the guide asks of the Merchant Center account — return
// policy, customer support — is not in the feed and cannot be checked here.

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/spagu/ssg/internal/models"
)

// Limits and vocabularies from Google's attribute pages (read 2026-11-14).
const (
	ucpNoticeMax     = 1000 // notice_message characters
	ucpOptionMax     = 250  // variant_option name and value characters
	ucpOptionsMax    = 30   // variant_option repeats per item
	ucpGroupTitleMax = 150  // item_group_title characters, truncated above
)

// ucpNoticeTypes are the notice_type values Google accepts.
var ucpNoticeTypes = map[string]bool{"legal_disclaimer": true, "safety_warning": true, "prop_65": true}

// ucpNoticeTagRe finds the tags in a notice; only b, br, i and a are shown.
var ucpNoticeTagRe = regexp.MustCompile(`</?\s*([A-Za-z][A-Za-z0-9]*)`)

// ucpFeedRecord is one Merchant feed written with `ucp:`: its path, its
// config, and every row the pages gave, the ones left out too.
type ucpFeedRecord struct {
	rel   string
	spec  models.FeedSpec
	items []merchantItem
}

// recordUCPFeed keeps a copy of a UCP feed's rows for check_ucp, and with the
// check off says in one line that there is something to see.
func (g *Generator) recordUCPFeed(rel string, spec models.FeedSpec, items []merchantItem) {
	rec := ucpFeedRecord{rel: rel, spec: spec, items: append([]merchantItem(nil), items...)}
	g.ucpFeeds = append(g.ucpFeeds, rec)
	if g.resolveMode(g.config.CheckUCP) != "" || g.config.Quiet {
		return
	}
	if written := ucpWritten(rec.items); written == 0 {
		g.emptyFeedNotice(rel, len(rec.items), written, "check_ucp")
	} else if n := len(ucpFindings(rec)); n > 0 {
		fmt.Printf("   ⚠️  %s: %d UCP problem(s) that may keep products out of checkout on Google; set check_ucp: warn for the list\n", rel, n)
	}
}

// checkUCPIfRequested reports the problems in every UCP feed this build
// wrote.
func (g *Generator) checkUCPIfRequested() error {
	mode := g.resolveMode(g.config.CheckUCP)
	if mode == "" || len(g.ucpFeeds) == 0 {
		return nil
	}
	g.log("🛒 Checking the UCP attributes of the Merchant feed...")
	var findings []finding
	for _, rec := range g.ucpFeeds {
		findings = append(findings, ucpFindings(rec)...)
	}
	sortFindings(findings)
	return g.report(findings, mode, "UCP feed",
		"UCP feed rows are eligible for checkout as written",
		"%d UCP feed problem(s)")
}

// ucpFindings checks one feed: each row, each variant group, and that some
// row is eligible at all.
func ucpFindings(rec ucpFeedRecord) []finding {
	var out []finding
	groups := map[string][]merchantItem{}
	var order []string
	eligible := false
	for _, it := range rec.items {
		out = append(out, ucpItemFindings(rec.rel, it)...)
		eligible = eligible || it.ucp.checkout
		if it.group != nil {
			if _, seen := groups[it.groupID]; !seen {
				order = append(order, it.groupID)
			}
			groups[it.groupID] = append(groups[it.groupID], it)
		}
	}
	for _, id := range order {
		out = append(out, ucpGroupFindings(rec.rel, groups[id])...)
	}
	written := ucpWritten(rec.items)
	if f, empty := emptyFeedFinding(rec.rel, rec.spec.Path, len(rec.items), written); empty {
		return append(out, f)
	}
	if o := rec.spec.UCP; !eligible && (o.CheckoutEligibility == nil || *o.CheckoutEligibility) {
		out = append(out, finding{"feeds: " + rec.spec.Path, "ucp: no row is eligible for checkout — every product is restricted or opted out"})
	}
	return out
}

// ucpWritten counts the rows the Merchant feed keeps: those with every
// attribute Google requires (keepAcceptedItems).
func ucpWritten(items []merchantItem) int {
	n := 0
	for _, it := range items {
		if len(it.missingRequired()) == 0 {
			n++
		}
	}
	return n
}

// ucpAdder returns the function that records one finding about a row.
func ucpAdder(rel string, it merchantItem, out *[]finding) func(field, msg string) {
	where := firstNonEmpty(it.source, it.page)
	return func(field, msg string) {
		*out = append(*out, finding{where, fmt.Sprintf("%s item %s: %s — %s", rel, it.id, field, msg)})
	}
}

// ucpItemFindings checks one row's own attributes.
func ucpItemFindings(rel string, it merchantItem) []finding {
	var out []finding
	add := ucpAdder(rel, it, &out)
	u := it.ucp
	if it.bareprice != "" {
		add("price", it.bareprice+" has no currency; set offers.priceCurrency — the row is left out")
	}
	for _, f := range it.missingRequired() {
		if f != "price" || it.bareprice == "" {
			add(f, "required and missing — the row is left out")
		}
	}
	if u.asked && u.restricted != "" {
		add("native_commerce", "written FALSE: Google lists "+u.restricted+" as ineligible for checkout")
	}
	if u.checkout && !it.stockFromPage {
		add("availability", it.availability+" is stated by no page; checkout sells what the feed says is in stock — set offers.availability or merchant.availability")
	}
	for _, n := range u.notices {
		for _, msg := range ucpNoticeProblems(n) {
			add("consumer_notice", msg)
		}
	}
	if it.group == nil {
		return out
	}
	if u.groupTitle == "" {
		add("item_group_title", "missing; set name on the ProductGroup — Google recommends it with variant_option")
	} else if utf8.RuneCountInString(u.groupTitle) > ucpGroupTitleMax {
		add("item_group_title", fmt.Sprintf("longer than %d characters; Google truncates it", ucpGroupTitleMax))
	}
	if len(u.options) == 0 {
		add("variant_option", "none; Google strongly recommends it on variants — set variesBy on the ProductGroup and that property on each variant")
	}
	if len(u.options) > ucpOptionsMax {
		add("variant_option", fmt.Sprintf("%d options; Google takes at most %d", len(u.options), ucpOptionsMax))
	}
	for _, o := range u.options {
		if utf8.RuneCountInString(o.name) > ucpOptionMax || utf8.RuneCountInString(o.value) > ucpOptionMax {
			add("variant_option", fmt.Sprintf("%s is longer than %d characters", o.name, ucpOptionMax))
		}
	}
	return out
}

// ucpNoticeProblems says what Google would not accept in a consumer notice.
func ucpNoticeProblems(n ucpNotice) []string {
	var out []string
	if !ucpNoticeTypes[n.kind] {
		out = append(out, fmt.Sprintf("notice_type %q is not legal_disclaimer, safety_warning or prop_65", n.kind))
	}
	switch {
	case n.message == "":
		out = append(out, "notice_message is empty")
	case utf8.RuneCountInString(n.message) > ucpNoticeMax:
		out = append(out, fmt.Sprintf("notice_message is longer than %d characters", ucpNoticeMax))
	}
	reported := map[string]bool{}
	for _, m := range ucpNoticeTagRe.FindAllStringSubmatch(n.message, -1) {
		tag := strings.ToLower(m[1])
		switch {
		case tag == "b" || tag == "br" || tag == "i" || tag == "a" || reported[tag]:
		default:
			reported[tag] = true
			out = append(out, "notice_message uses <"+tag+">; only <b>, <br>, <i> and <a href> are shown")
		}
	}
	return out
}

// ucpGroupFindings checks the variants of one group together: Google wants
// the same option names on each, and a different combination of values.
func ucpGroupFindings(rel string, items []merchantItem) []finding {
	var out []finding
	names := ucpOptionNames(items[0].ucp.options)
	seen := map[string]string{}
	for _, it := range items {
		add := ucpAdder(rel, it, &out)
		if got := ucpOptionNames(it.ucp.options); got != names {
			add("variant_option", fmt.Sprintf("names %q differ from %q on %s; every variant of a group needs the same ones", got, names, items[0].id))
		}
		var combo []string
		for _, o := range it.ucp.options {
			combo = append(combo, o.name+":"+o.value)
		}
		key := strings.Join(combo, ",")
		if first, dup := seen[key]; dup && key != "" {
			add("variant_option", key+" is also "+first+"'s; each variant needs its own combination")
			continue
		}
		seen[key] = it.id
	}
	return out
}

// ucpOptionNames is a variant's option names, joined, for comparison.
func ucpOptionNames(opts []variantOption) string {
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.name
	}
	return strings.Join(names, ",")
}
