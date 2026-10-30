package generator

// The Google Merchant Center product feed (#196).

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func merchantGen(t *testing.T, pages, posts []models.Page) (*Generator, string) {
	t.Helper()
	out := t.TempDir()
	g, err := New(Config{Domain: "example.com", OutputDir: out, Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData = &models.SiteData{Pages: pages, Posts: posts}
	return g, out
}

func productPage(slug string, schema map[string]interface{}, extra map[string]interface{}) models.Page {
	return models.Page{Title: strings.ToUpper(slug), Slug: slug, Type: "page", Description: "About " + slug,
		SourceDir: "shop", Schema: schema, Extra: extra}
}

var mug = map[string]interface{}{
	"@type": "Product", "name": "Mug", "sku": "MUG-1", "image": "/img/mug.jpg",
	"brand":  map[string]interface{}{"@type": "Brand", "name": "Example"},
	"gtin13": "5901234123457",
	"offers": map[string]interface{}{"@type": "Offer", "price": 12.5, "priceCurrency": "pln",
		"availability": "https://schema.org/OutOfStock", "itemCondition": "https://schema.org/UsedCondition"},
}

func writeMerchant(t *testing.T, g *Generator, out string, spec models.FeedSpec) string {
	t.Helper()
	spec.Format = merchantFormat
	if err := g.writeDeclaredFeed(spec); err != nil {
		t.Fatal(err)
	}
	return mustRead(t, filepath.Join(out, strings.TrimPrefix(spec.Path, "/")))
}

// TestMerchantFeedFromProductSchema: a page's Product + Offer become one row
// in Google's words, and the file is well-formed XML.
func TestMerchantFeedFromProductSchema(t *testing.T) {
	g, out := merchantGen(t, []models.Page{productPage("mug", mug, nil), {Title: "About", Slug: "about"}}, nil)
	feed := writeMerchant(t, g, out, models.FeedSpec{Path: "/merchant.xml", Title: "Shop"})
	wantContains(t, "feed", feed,
		`<rss version="2.0" xmlns:g="http://base.google.com/ns/1.0">`,
		"<g:id>MUG-1</g:id>", "<g:title>Mug</g:title>", "<g:description>About mug</g:description>",
		"<g:link>https://example.com/mug/</g:link>", "<g:image_link>https://example.com/img/mug.jpg</g:image_link>",
		"<g:price>12.5 PLN</g:price>", "<g:availability>out_of_stock</g:availability>",
		"<g:condition>used</g:condition>", "<g:brand>Example</g:brand>", "<g:gtin>5901234123457</g:gtin>")
	if strings.Count(feed, "<item>") != 1 {
		t.Errorf("only the product page is a row:\n%s", feed)
	}
	if err := xml.Unmarshal([]byte(feed), new(struct{})); err != nil {
		t.Errorf("not XML: %v", err)
	}
}

// TestMerchantFeedVariants: a ProductGroup is one row per variant, sharing an
// item_group_id and inheriting what the variant leaves out.
func TestMerchantFeedVariants(t *testing.T) {
	group := map[string]interface{}{
		"@type": "ProductGroup", "name": "Shirt", "productGroupID": "SHIRT", "image": []interface{}{map[string]interface{}{"url": "/shirt.jpg"}},
		"offers": map[string]interface{}{"price": "20", "priceCurrency": "EUR", "availability": "https://schema.org/PreOrder"},
		"hasVariant": []interface{}{
			map[string]interface{}{"@type": "Product", "sku": "SHIRT-S", "name": "Shirt S"},
			map[string]interface{}{"@type": "Product", "sku": "SHIRT-M", "offers": map[string]interface{}{"price": "22", "priceCurrency": "EUR", "availability": "BackOrder"}},
		},
	}
	g, out := merchantGen(t, []models.Page{productPage("shirt", group, map[string]interface{}{"merchant": map[string]interface{}{"id": "ignored", "brand": "Front"}})}, nil)
	feed := writeMerchant(t, g, out, models.FeedSpec{Path: "/merchant.xml"})
	wantContains(t, "feed", feed, "<g:id>SHIRT-S</g:id>", "<g:id>SHIRT-M</g:id>", "<g:item_group_id>SHIRT</g:item_group_id>",
		"<g:title>Shirt S</g:title>", "<g:title>Shirt</g:title>", "<g:image_link>https://example.com/shirt.jpg</g:image_link>",
		"<g:availability>preorder</g:availability>", "<g:availability>backorder</g:availability>", "<g:price>22 EUR</g:price>",
		"<g:brand>Front</g:brand>")
	if strings.Contains(feed, "ignored") {
		t.Error("a front-matter id must not be given to every variant")
	}
}

// TestMerchantFeedIDsAndDefaults: no sku falls back to merchant.id, then to
// the URL; the feed's defaults fill what the page does not say; a row Google
// would reject is left out; a graph is searched; a post can be a product.
func TestMerchantFeedIDsAndDefaults(t *testing.T) {
	noSKU := map[string]interface{}{"@type": []interface{}{"Product", "Thing"}, "name": "Lamp", "image": "/lamp.jpg",
		"offers": []interface{}{map[string]interface{}{"price": 99, "priceCurrency": "USD"}}}
	graph := map[string]interface{}{"@graph": []interface{}{"junk", map[string]interface{}{"@type": "Product", "name": "Desk",
		"image": map[string]interface{}{"contentUrl": "/desk.jpg"}, "offers": map[string]interface{}{"price": "300", "priceCurrency": "USD"}}}}
	broken := map[string]interface{}{"@type": "Product", "name": "Ghost", "sku": "GHOST"}
	pages := []models.Page{
		productPage("lamp", noSKU, map[string]interface{}{"merchant": map[string]interface{}{"id": "LAMP-7", "condition": "refurbished"}}),
		productPage("desk", graph, nil),
		productPage("ghost", broken, nil),
	}
	review := models.Page{Title: "Kettle review", Slug: "kettle", Type: "post", Description: "Kettle", SourceDir: "shop",
		Schema: map[string]interface{}{"@type": "Product", "sku": "KET", "image": "/k.jpg", "offers": map[string]interface{}{"price": 5, "priceCurrency": "USD"}}}
	g, out := merchantGen(t, pages, []models.Page{review})
	g.config.Quiet = false
	feed := writeMerchant(t, g, out, models.FeedSpec{Path: "/shop/merchant.xml", Source: "shop",
		Merchant: models.MerchantDefaults{Availability: "in_stock", Brand: "House"}})
	wantContains(t, "feed", feed, "<g:id>LAMP-7</g:id>", "<g:condition>refurbished</g:condition>",
		"<g:id>/desk/</g:id>", "<g:image_link>https://example.com/desk.jpg</g:image_link>",
		"<g:brand>House</g:brand>", "<g:id>KET</g:id>", "<g:title>Kettle review</g:title>")
	if strings.Contains(feed, "GHOST") {
		t.Error("a product without image or price is rejected by Google and left out")
	}
	// type: page leaves the post out, and items caps the catalogue.
	one := 1
	pagesOnly := writeMerchant(t, g, out, models.FeedSpec{Path: "/p.xml", Type: "page", Items: &one})
	if strings.Count(pagesOnly, "<item>") != 1 || strings.Contains(pagesOnly, "KET") {
		t.Errorf("type: page and items: 1:\n%s", pagesOnly)
	}
}

// TestMerchantFeedNotAdvertised: the product feed is for Merchant Center, so
// pages do not offer it to feed readers.
func TestMerchantFeedNotAdvertised(t *testing.T) {
	g, _ := merchantGen(t, nil, nil)
	g.config.Feeds = []models.FeedSpec{{Path: "/merchant.xml", Format: "google-merchant"}, {Path: "/blog.xml"}}
	links := g.feedAutodiscoveryLinks("")
	if strings.Contains(links, "merchant.xml") || !strings.Contains(links, "blog.xml") {
		t.Errorf("links = %s", links)
	}
}

func TestMerchantMappings(t *testing.T) {
	for in, want := range map[string]string{"": "in_stock", "https://schema.org/InStock": "in_stock", "SoldOut": "out_of_stock",
		"out_of_stock": "out_of_stock", "https://schema.org/Discontinued": "out_of_stock", "PreSale": "preorder"} {
		if got := merchantAvailability(in); got != want {
			t.Errorf("availability %q = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"": "", "https://schema.org/NewCondition": "", "DamagedCondition": "used", "RefurbishedCondition": "refurbished"} {
		if got := merchantCondition(in); got != want {
			t.Errorf("condition %q = %q, want %q", in, got, want)
		}
	}
	if ldName("Plain") != "Plain" || ldImage(nil) != "" || firstLD(nil, []interface{}{}) != nil {
		t.Error("helpers on plain values")
	}
}

func TestMerchantFeedUnwritable(t *testing.T) {
	g, out := merchantGen(t, []models.Page{productPage("mug", mug, nil)}, nil)
	mustWrite(t, filepath.Join(out, "blocked"), "x")
	err := g.writeDeclaredFeed(models.FeedSpec{Path: "/blocked/merchant.xml", Format: merchantFormat})
	if err == nil {
		t.Error("a file in the way must fail")
	}
	if _, statErr := os.Stat(filepath.Join(out, "blocked", "merchant.xml")); statErr == nil {
		t.Error("nothing written")
	}
}
