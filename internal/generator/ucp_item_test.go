package generator

// UCP attributes on the Google Merchant feed (GO-166).

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// ucpOn is a `ucp:` key with nothing set: checkout eligible, cart left out.
func ucpOn() *models.UCPOptions { return &models.UCPOptions{} }

// boolPtr is a *bool for a literal.
func boolPtr(v bool) *bool { return &v }

// ucpElementsRe matches every element the UCP switch adds.
var ucpElementsRe = regexp.MustCompile(`(?s)<g:(item_group_title|native_commerce|merchant_item_id|variant_option|consumer_notice)>.*?</g:(item_group_title|native_commerce|merchant_item_id|variant_option|consumer_notice)>\n`)

// ucpItem is the <item> of one id in a Merchant feed.
func ucpItem(t *testing.T, feed, id string) string {
	t.Helper()
	start := strings.Index(feed, "<g:id>"+id+"</g:id>")
	if start < 0 {
		t.Fatalf("no item %s in\n%s", id, feed)
	}
	return feed[start : start+strings.Index(feed[start:], "</item>")]
}

var kettle = map[string]interface{}{
	"@type": "Product", "name": "Kettle", "sku": "KETTLE", "image": "/k.jpg",
	"offers": map[string]interface{}{"price": "49.00", "priceCurrency": "USD", "availability": "https://schema.org/InStock"},
}

var ucpTee = map[string]interface{}{
	"@type": "ProductGroup", "name": "Tee", "productGroupID": "TEE", "image": "/t.jpg",
	"variesBy": []interface{}{"https://schema.org/size", "https://schema.org/color"},
	"offers":   map[string]interface{}{"price": "25", "priceCurrency": "USD", "availability": "InStock"},
	"hasVariant": []interface{}{
		map[string]interface{}{"@type": "Product", "sku": "TEE-S", "size": "S", "color": "red"},
		map[string]interface{}{"@type": "Product", "sku": "TEE-M", "size": map[string]interface{}{"name": "M"}, "color": "blue"},
	},
}

// TestUCPAttributes: each attribute, one case per way it is set.
func TestUCPAttributes(t *testing.T) {
	notice := []interface{}{map[string]interface{}{"notice_type": "prop_65", "notice_message": "  Can expose you to <b>lead</b> & more.  "}}
	for _, tc := range []struct {
		name   string
		schema map[string]interface{}
		front  map[string]interface{}
		opts   *models.UCPOptions
		id     string
		want   []string
		absent []string
	}{
		{name: "default is checkout TRUE, no cart", schema: kettle, opts: ucpOn(), id: "KETTLE",
			want:   []string{"<g:native_commerce>\n<g:checkout_eligibility>TRUE</g:checkout_eligibility>\n</g:native_commerce>\n"},
			absent: []string{"cart_eligibility", "merchant_item_id", "variant_option", "consumer_notice", "item_group_title"}},
		{name: "feed turns checkout off and states cart", schema: kettle, id: "KETTLE",
			opts: &models.UCPOptions{CheckoutEligibility: boolPtr(false), CartEligibility: boolPtr(true)},
			want: []string{"<g:checkout_eligibility>FALSE</g:checkout_eligibility>\n<g:cart_eligibility>TRUE</g:cart_eligibility>"}},
		{name: "page overrides both", schema: kettle, opts: &models.UCPOptions{CartEligibility: boolPtr(true)}, id: "KETTLE",
			front: map[string]interface{}{"checkout_eligibility": false, "cart_eligibility": false},
			want:  []string{"<g:checkout_eligibility>FALSE</g:checkout_eligibility>\n<g:cart_eligibility>FALSE</g:cart_eligibility>"}},
		{name: "merchant_item_id", schema: kettle, opts: ucpOn(), id: "KETTLE",
			front: map[string]interface{}{"merchant_item_id": "kettle-api-1"},
			want:  []string{"<g:merchant_item_id>kettle-api-1</g:merchant_item_id>"}},
		{name: "merchant_item_id equal to id is left out", schema: kettle, opts: ucpOn(), id: "KETTLE",
			front: map[string]interface{}{"merchant_item_id": "KETTLE"}, absent: []string{"merchant_item_id"}},
		{name: "consumer notice keeps its markup, escaped", schema: kettle, opts: ucpOn(), id: "KETTLE",
			front: map[string]interface{}{"consumer_notice": notice},
			want: []string{"<g:consumer_notice>\n<g:notice_type>prop_65</g:notice_type>\n" +
				"<g:notice_message>Can expose you to &lt;b&gt;lead&lt;/b&gt; &amp; more.</g:notice_message>\n</g:consumer_notice>"}},
		{name: "one notice as a map, junk skipped", schema: kettle, opts: ucpOn(), id: "KETTLE",
			front: map[string]interface{}{"consumer_notice": []interface{}{"junk", map[string]interface{}{"notice_type": "legal_disclaimer", "notice_message": "Read this."}}},
			want:  []string{"<g:notice_type>legal_disclaimer</g:notice_type>"}},
		{name: "variants", schema: ucpTee, opts: ucpOn(), id: "TEE-M",
			want: []string{"<g:item_group_title>Tee</g:item_group_title>",
				"<g:variant_option>\n<g:name>size</g:name>\n<g:value>M</g:value>\n</g:variant_option>\n<g:variant_option>\n<g:name>color</g:name>\n<g:value>blue</g:value>\n</g:variant_option>"},
			absent: []string{"merchant_item_id"}},
		{name: "variants ignore a page's merchant_item_id", schema: ucpTee, opts: ucpOn(), id: "TEE-S",
			front: map[string]interface{}{"merchant_item_id": "x"}, absent: []string{"merchant_item_id"}},
		{name: "pre-order is written FALSE", schema: withOffer(kettle, "availability", "https://schema.org/PreOrder"), opts: ucpOn(), id: "KETTLE",
			want: []string{"<g:checkout_eligibility>FALSE</g:checkout_eligibility>"}},
		{name: "used is written FALSE", schema: kettle, opts: ucpOn(), id: "KETTLE",
			front: map[string]interface{}{"condition": "used"}, want: []string{"<g:checkout_eligibility>FALSE</g:checkout_eligibility>"}},
		{name: "final sale is written FALSE", id: "KETTLE", opts: ucpOn(),
			schema: withOffer(kettle, "hasMerchantReturnPolicy", map[string]interface{}{"returnPolicyCategory": "https://schema.org/MerchantReturnNotPermitted"}),
			want:   []string{"<g:checkout_eligibility>FALSE</g:checkout_eligibility>"}},
		{name: "a return window is not a final sale", id: "KETTLE", opts: ucpOn(),
			schema: withOffer(kettle, "hasMerchantReturnPolicy", map[string]interface{}{"returnPolicyCategory": "https://schema.org/MerchantReturnFiniteReturnWindow"}),
			want:   []string{"<g:checkout_eligibility>TRUE</g:checkout_eligibility>"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var extra map[string]interface{}
			if tc.front != nil {
				extra = map[string]interface{}{"merchant": tc.front}
			}
			g, out := merchantGen(t, []models.Page{productPage("p", tc.schema, extra)}, nil)
			feed := writeMerchant(t, g, out, models.FeedSpec{Path: "/products.xml", UCP: tc.opts})
			if err := xml.Unmarshal([]byte(feed), new(interface{})); err != nil {
				t.Fatalf("not well-formed XML: %v\n%s", err, feed)
			}
			item := ucpItem(t, feed, tc.id)
			for _, w := range tc.want {
				if !strings.Contains(item, w) {
					t.Errorf("missing %q in\n%s", w, item)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(item, a) {
					t.Errorf("unexpected %q in\n%s", a, item)
				}
			}
		})
	}
}

// withOffer is a copy of a product with one key of its offer replaced.
func withOffer(p map[string]interface{}, key string, value interface{}) map[string]interface{} {
	out, offer := map[string]interface{}{}, map[string]interface{}{}
	for k, v := range p {
		out[k] = v
	}
	for k, v := range p["offers"].(map[string]interface{}) {
		offer[k] = v
	}
	offer[key] = value
	out["offers"] = offer
	return out
}

// TestUCPOffIsByteIdentical: without `ucp:` nothing UCP is written, and the
// feed with it, minus the UCP elements, is the same bytes.
func TestUCPOffIsByteIdentical(t *testing.T) {
	front := map[string]interface{}{"merchant": map[string]interface{}{"merchant_item_id": "k-1", "checkout_eligibility": true,
		"consumer_notice": map[string]interface{}{"notice_type": "prop_65", "notice_message": "Lead."}}}
	pages := []models.Page{productPage("kettle", kettle, front), productPage("tee", ucpTee, nil), productPage("mug", mug, nil)}
	g, out := merchantGen(t, pages, nil)
	off := writeMerchant(t, g, out, models.FeedSpec{Path: "/off.xml"})
	on := writeMerchant(t, g, out, models.FeedSpec{Path: "/on.xml", UCP: &models.UCPOptions{CartEligibility: boolPtr(true)}})
	if ucpElementsRe.MatchString(off) {
		t.Errorf("a feed without ucp: has UCP elements:\n%s", off)
	}
	if stripped := ucpElementsRe.ReplaceAllString(on, ""); stripped != off || on == off {
		t.Errorf("ucp: changed more than its own elements:\n%s\n---\n%s", off, on)
	}
	if len(g.ucpFeeds) != 1 {
		t.Errorf("only the ucp: feed is recorded for check_ucp, got %d", len(g.ucpFeeds))
	}
}

// TestUCPFeedDeterministic: the same products in any page order give the
// same bytes.
func TestUCPFeedDeterministic(t *testing.T) {
	pages := []models.Page{productPage("kettle", kettle, nil), productPage("tee", ucpTee, nil), productPage("mug", mug, nil)}
	g, out := merchantGen(t, pages, nil)
	first := writeMerchant(t, g, out, models.FeedSpec{Path: "/p.xml", UCP: ucpOn()})
	g.siteData.Pages = []models.Page{pages[2], pages[1], pages[0]}
	for i := 0; i < 5; i++ {
		if again := writeMerchant(t, g, out, models.FeedSpec{Path: "/p.xml", UCP: ucpOn()}); again != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first, again)
		}
	}
}

// TestVariantOptions: variesBy order, the color/size fallback, and values the
// variant leaves out.
func TestVariantOptions(t *testing.T) {
	for _, tc := range []struct {
		variant, group map[string]interface{}
		want           string
	}{
		{map[string]interface{}{"size": "M", "color": "blue", "material": "linen"},
			map[string]interface{}{"variesBy": []interface{}{"https://schema.org/material", "size"}}, "material:linen,size:M"},
		{map[string]interface{}{"size": "M", "color": "blue"}, map[string]interface{}{}, "color:blue,size:M"},
		{map[string]interface{}{"size": "M"}, map[string]interface{}{"variesBy": "https://schema.org/color"}, ""},
	} {
		var got []string
		for _, o := range variantOptions(tc.variant, tc.group) {
			got = append(got, o.name+":"+o.value)
		}
		if strings.Join(got, ",") != tc.want {
			t.Errorf("variantOptions(%v, %v) = %v, want %s", tc.variant, tc.group, got, tc.want)
		}
	}
	if acpVariantDict(map[string]interface{}{}, map[string]interface{}{}) != nil {
		t.Error("no options is a nil variant_dict")
	}
}
