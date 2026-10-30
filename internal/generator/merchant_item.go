package generator

// Schema.org Product data → Merchant Center attributes (#196).

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// productItem reads one Product node. Schema.org states most of a feed row;
// availability and condition are the two Google spells differently.
func (g *Generator) productItem(node map[string]interface{}, page models.Page, defaults models.MerchantDefaults) merchantItem {
	offer := ldMap(firstLD(node["offers"]))
	item := merchantItem{
		id:           ldText(node["sku"]),
		title:        firstNonEmpty(ldText(node["name"]), page.Title),
		description:  firstNonEmpty(ldText(node["description"]), page.Description),
		link:         g.socialURL(firstNonEmpty(ldText(node["url"]), ldText(offer["url"]), page.GetCanonical(g.config.Domain))),
		image:        g.socialURL(ldImage(node["image"])),
		availability: firstNonEmpty(ldText(offer["availability"]), defaults.Availability),
		condition:    firstNonEmpty(ldText(firstLD(node["itemCondition"], offer["itemCondition"])), defaults.Condition),
		brand:        firstNonEmpty(ldName(node["brand"]), defaults.Brand),
		gtin:         ldText(firstLD(node["gtin"], node["gtin13"], node["gtin12"], node["gtin14"], node["gtin8"])),
		mpn:          ldText(node["mpn"]),
	}
	if price, currency := ldText(offer["price"]), ldText(offer["priceCurrency"]); price != "" && currency != "" {
		item.price = price + " " + strings.ToUpper(currency)
	}
	return item
}

// merchantAvailability maps a Schema.org ItemAvailability, or a value already
// in Google's words, to Google's. Unset is in_stock: a product page that says
// nothing about stock is a page selling it.
func merchantAvailability(v string) string {
	switch strings.ToLower(ldTerm(v)) {
	case "outofstock", "out_of_stock", "soldout", "discontinued":
		return "out_of_stock"
	case "preorder", "presale":
		return "preorder"
	case "backorder":
		return "backorder"
	}
	return "in_stock"
}

// merchantCondition maps a Schema.org OfferItemCondition. New is Google's
// default and is left out, as the spec asks.
func merchantCondition(v string) string {
	switch strings.ToLower(ldTerm(v)) {
	case "usedcondition", "used", "damagedcondition":
		return "used"
	case "refurbishedcondition", "refurbished":
		return "refurbished"
	}
	return ""
}

// ldTerm is the last segment of a Schema.org enumeration URL:
// "https://schema.org/InStock" → "InStock".
func ldTerm(v string) string {
	return v[strings.LastIndex(v, "/")+1:]
}

// ldNodes are the nodes of a merged schema: the root and its @graph.
func ldNodes(root map[string]interface{}) []map[string]interface{} {
	out := []map[string]interface{}{root}
	for _, n := range ldList(root["@graph"]) {
		if m := ldMap(n); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// ldHasType reports whether a node's @type, one name or a list, includes t.
func ldHasType(node map[string]interface{}, t string) bool {
	for _, v := range ldList(node["@type"]) {
		if ldText(v) == t {
			return true
		}
	}
	return false
}

// ldMap is a value as an object, or nil.
func ldMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// firstLD is the first item of the first value that has one.
func firstLD(values ...interface{}) interface{} {
	for _, v := range values {
		if list := ldList(v); len(list) > 0 {
			return list[0]
		}
	}
	return nil
}

// ldImage is an image given as a URL, a list or an ImageObject.
func ldImage(v interface{}) string {
	first := firstLD(v)
	if m := ldMap(first); m != nil {
		return ldText(firstLD(m["url"], m["contentUrl"]))
	}
	return ldText(first)
}

// ldName is a name given as text or as an object with one (a Brand).
func ldName(v interface{}) string {
	if m := ldMap(v); m != nil {
		return ldText(m["name"])
	}
	return ldText(v)
}

// inheritLD is a variant with what it leaves out taken from its group.
func inheritLD(variant, group map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{"name", "description", "image", "brand", "offers", "url", "itemCondition"} {
		if v, ok := group[key]; ok {
			out[key] = v
		}
	}
	for k, v := range variant {
		out[k] = v
	}
	return out
}

// renderMerchantFeed writes the rows as RSS 2.0 in the Google namespace.
func (g *Generator) renderMerchantFeed(title string, items []merchantItem) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<rss version="2.0" xmlns:g="http://base.google.com/ns/1.0">` + "\n<channel>\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<link>%s</link>\n<description>%s</description>\n",
		xmlText(title), xmlText(httpsScheme+g.config.Domain+"/"), xmlText(title))
	for _, it := range items {
		b.WriteString("<item>\n")
		for _, f := range []struct{ name, value string }{
			{"id", it.id}, {"item_group_id", it.groupID}, {"title", it.title},
			{"description", it.description}, {"link", it.link}, {"image_link", it.image},
			{"price", it.price}, {"availability", it.availability}, {"condition", it.condition},
			{"brand", it.brand}, {"gtin", it.gtin}, {"mpn", it.mpn},
		} {
			if f.value != "" {
				fmt.Fprintf(&b, "<g:%s>%s</g:%s>\n", f.name, xmlText(f.value), f.name)
			}
		}
		b.WriteString("</item>\n")
	}
	b.WriteString("</channel>\n</rss>\n")
	return b.String()
}

// xmlText escapes a value for XML character data.
func xmlText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s)) // a bytes.Buffer cannot fail
	return buf.String()
}
