package generator

// Google's Universal Commerce Protocol on the Merchant feed (GO-166).
//
// UCP has no product file of its own: the protocol (ucp.dev) is a set of
// live APIs — checkout, cart, catalog — that a shop serves, and Google reads
// the products an agent may buy from Merchant Center. What a site states for
// UCP is therefore a handful of extra attributes on the Merchant feed it
// already has (#196), as Google's guide lists them (read 2026-11-14,
// developers.google.com/merchant/ucp/guides/overview/merchant-center):
//
//   - native_commerce, a group of checkout_eligibility (default FALSE) and
//     cart_eligibility (opt-out, default TRUE). The group is overwritten as a
//     whole on every upload, so every row carries every flag the feed uses.
//   - variant_option (name/value, up to 30) with item_group_title, "strongly
//     recommended" for variants.
//   - consumer_notice (notice_type + notice_message), mandatory for products
//     with a regulatory warning.
//   - merchant_item_id, when the checkout knows a product by another id.
//
// All of it is written only when the feed entry has a `ucp:` key; without it
// the feed is byte-for-byte what it was. Return policy and customer support
// are Merchant Center account settings, not feed attributes, and stay the
// merchant's to set there.

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// ucpRow is the UCP part of one Merchant feed row.
type ucpRow struct {
	// asked is the checkout eligibility the feed and page ask for; checkout
	// is what is written: asked, unless the product is restricted.
	asked, checkout bool
	cart            *bool
	itemID          string
	groupTitle      string
	options         []variantOption
	notices         []ucpNotice
	// restricted names why Google lists the product as ineligible for
	// checkout, or "".
	restricted string
}

// variantOption is one dimension of a variant and its value: size → M.
type variantOption struct{ name, value string }

// ucpNotice is one consumer_notice: its notice_type and notice_message.
type ucpNotice struct{ kind, message string }

// ucpRowOf builds the UCP attributes of one row: the feed's values, the
// page's `merchant:` keys over them, the variant data from the JSON-LD.
func ucpRowOf(it merchantItem, o models.UCPOptions) *ucpRow {
	front := ldMap(it.front["merchant"])
	u := &ucpRow{asked: o.CheckoutEligibility == nil || *o.CheckoutEligibility, cart: o.CartEligibility}
	if v, ok := front["checkout_eligibility"].(bool); ok {
		u.asked = v
	}
	if v, ok := front["cart_eligibility"].(bool); ok {
		u.cart = &v
	}
	if id := ldText(front["merchant_item_id"]); it.single && id != it.id {
		u.itemID = id
	}
	if it.group != nil {
		u.groupTitle = ldText(it.group["name"])
		u.options = variantOptions(it.node, it.group)
	}
	u.notices = ucpNotices(front["consumer_notice"])
	u.restricted = ucpRestriction(it)
	u.checkout = u.asked && u.restricted == ""
	return u
}

// ucpNotices reads `consumer_notice:` — one {notice_type, notice_message} or
// a list of them. The message keeps its markup: Google shows <b>, <br>, <i>
// and <a href> as written.
func ucpNotices(v interface{}) []ucpNotice {
	var out []ucpNotice
	for _, n := range ldList(v) {
		m := ldMap(n)
		if m == nil {
			continue
		}
		msg, _ := m["notice_message"].(string)
		out = append(out, ucpNotice{kind: ldText(m["notice_type"]), message: strings.TrimSpace(msg)})
	}
	return out
}

// ucpRestriction names the reason Google's guide lists a product as
// ineligible for checkout, as far as its data shows one: a pre-order, a used
// or refurbished item, a final sale (a return policy of no returns).
func ucpRestriction(it merchantItem) string {
	switch {
	case it.availability == "preorder":
		return "pre-order items"
	case it.condition != "":
		return it.condition + " items"
	}
	offer := ldMap(firstLD(it.node["offers"]))
	policy := ldMap(firstLD(offer["hasMerchantReturnPolicy"], it.node["hasMerchantReturnPolicy"]))
	if ldTerm(ldText(policy["returnPolicyCategory"])) == "MerchantReturnNotPermitted" {
		return "final-sale items (no returns)"
	}
	return ""
}

// variantOptions are a variant's options in a stable order: the properties
// its group varies by (variesBy), else color and size. An option the variant
// gives no value for is left out.
func variantOptions(variant, group map[string]interface{}) []variantOption {
	keys := []string{"color", "size"}
	if by := ldTexts(group["variesBy"]); len(by) > 0 {
		keys = keys[:0]
		for _, k := range by {
			keys = append(keys, ldTerm(k))
		}
	}
	var out []variantOption
	for _, k := range keys {
		if v := ldName(variant[k]); v != "" {
			out = append(out, variantOption{name: k, value: v})
		}
	}
	return out
}

// ucpFlag is a boolean in Merchant Center's spelling.
func ucpFlag(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}

// writeXML writes the row's UCP elements in the Google namespace, the shape
// of the guide's own XML example.
func (u *ucpRow) writeXML(b *strings.Builder) {
	if u.groupTitle != "" {
		fmt.Fprintf(b, "<g:item_group_title>%s</g:item_group_title>\n", xmlText(u.groupTitle))
	}
	fmt.Fprintf(b, "<g:native_commerce>\n<g:checkout_eligibility>%s</g:checkout_eligibility>\n", ucpFlag(u.checkout))
	if u.cart != nil {
		fmt.Fprintf(b, "<g:cart_eligibility>%s</g:cart_eligibility>\n", ucpFlag(*u.cart))
	}
	b.WriteString("</g:native_commerce>\n")
	if u.itemID != "" {
		fmt.Fprintf(b, "<g:merchant_item_id>%s</g:merchant_item_id>\n", xmlText(u.itemID))
	}
	for _, o := range u.options {
		fmt.Fprintf(b, "<g:variant_option>\n<g:name>%s</g:name>\n<g:value>%s</g:value>\n</g:variant_option>\n", xmlText(o.name), xmlText(o.value))
	}
	for _, n := range u.notices {
		fmt.Fprintf(b, "<g:consumer_notice>\n<g:notice_type>%s</g:notice_type>\n<g:notice_message>%s</g:notice_message>\n</g:consumer_notice>\n",
			xmlText(n.kind), xmlText(n.message))
	}
}
