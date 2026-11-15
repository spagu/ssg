package generator

// check_ucp (GO-166) and a fixture site's Merchant feed, with and without
// `ucp:`, against golden files.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

var updateUCPGolden = flag.Bool("update-ucp", false, "rewrite testdata/ucp/*.xml")

// ucpFindingsOf writes a UCP Merchant feed of the pages and returns
// check_ucp's findings as "where | message" lines.
func ucpFindingsOf(t *testing.T, opts *models.UCPOptions, pages ...models.Page) []string {
	t.Helper()
	g, out := merchantGen(t, pages, nil)
	writeMerchant(t, g, out, models.FeedSpec{Path: "/products.xml", UCP: opts})
	var lines []string
	for _, f := range ucpFindings(g.ucpFeeds[0]) {
		lines = append(lines, f.file+" | "+f.detail)
	}
	return lines
}

// TestCheckUCPFindings: every finding, each naming the source file and the
// field, and a clean row with none.
func TestCheckUCPFindings(t *testing.T) {
	long := strings.Repeat("x", ucpOptionMax+1)
	many := map[string]interface{}{"@type": "Product", "sku": "MANY"}
	var by []interface{}
	for i := 0; i <= ucpOptionsMax; i++ {
		key := fmt.Sprintf("p%d", i)
		by, many[key] = append(by, key), "v"
	}
	for _, tc := range []struct {
		name   string
		schema map[string]interface{}
		front  map[string]interface{}
		want   []string
	}{
		{name: "clean", schema: kettle},
		{name: "bare price", schema: withOffer(kettle, "priceCurrency", ""),
			want: []string{"price — 49.00 has no currency"}},
		{name: "missing required", schema: map[string]interface{}{"@type": "Product", "sku": "NOIMG", "offers": map[string]interface{}{"availability": "InStock"}},
			want: []string{"image_link — required and missing", "price — required and missing"}},
		{name: "restricted", schema: withOffer(kettle, "availability", "PreOrder"),
			want: []string{"native_commerce — written FALSE: Google lists pre-order items as ineligible", "no row is eligible"}},
		{name: "restricted but not asked", schema: withOffer(kettle, "availability", "PreOrder"),
			front: map[string]interface{}{"checkout_eligibility": false}, want: []string{"no row is eligible"}},
		{name: "stock unstated", schema: withOffer(kettle, "availability", ""),
			want: []string{"availability — in_stock is stated by no page"}},
		{name: "notices", schema: kettle, front: map[string]interface{}{"consumer_notice": []interface{}{
			map[string]interface{}{"notice_type": "health", "notice_message": "<p>Read</p> <B>this</B> <br/> <a href=x>here</a>"},
			map[string]interface{}{"notice_type": "prop_65"},
			map[string]interface{}{"notice_type": "safety_warning", "notice_message": strings.Repeat("y", ucpNoticeMax+1)},
		}}, want: []string{`notice_type "health" is not`, "uses <p>; only", "notice_message is empty", "longer than 1000 characters"}},
		{name: "variants without options or title", schema: map[string]interface{}{"@graph": []interface{}{map[string]interface{}{"@type": "ProductGroup", "sku": "G",
			"offers":     map[string]interface{}{"price": "1", "priceCurrency": "USD", "availability": "InStock"},
			"hasVariant": []interface{}{map[string]interface{}{"sku": "G-1", "name": "One", "image": "/1.jpg"}}}}},
			want: []string{"item_group_title — missing", "variant_option — none"}},
		{name: "long title and options", schema: map[string]interface{}{"@type": "ProductGroup", "sku": "L", "name": strings.Repeat("t", ucpGroupTitleMax+1),
			"image": "/l.jpg", "variesBy": "size", "offers": map[string]interface{}{"price": "1", "priceCurrency": "USD", "availability": "InStock"},
			"hasVariant": []interface{}{map[string]interface{}{"sku": "L-1", "size": long}}},
			want: []string{"item_group_title — longer than 150", "variant_option — size is longer than 250"}},
		{name: "too many options", schema: map[string]interface{}{"@type": "ProductGroup", "sku": "M", "name": "Many", "image": "/m.jpg", "variesBy": by,
			"offers": map[string]interface{}{"price": "1", "priceCurrency": "USD", "availability": "InStock"}, "hasVariant": []interface{}{many}},
			want: []string{"variant_option — 31 options; Google takes at most 30"}},
		{name: "group names and combinations", schema: map[string]interface{}{"@type": "ProductGroup", "sku": "T", "name": "Tee", "image": "/t.jpg",
			"variesBy": []interface{}{"size", "color"}, "offers": map[string]interface{}{"price": "1", "priceCurrency": "USD", "availability": "InStock"},
			"hasVariant": []interface{}{
				map[string]interface{}{"sku": "T-1", "size": "S", "color": "red"},
				map[string]interface{}{"sku": "T-2", "size": "S", "color": "red"},
				map[string]interface{}{"sku": "T-3", "size": "M"},
			}},
			want: []string{`T-2: variant_option — size:S,color:red is also T-1's`, `T-3: variant_option — names "size" differ from "size,color" on T-1`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var extra map[string]interface{}
			if tc.front != nil {
				extra = map[string]interface{}{"merchant": tc.front}
			}
			p := productPage("p", tc.schema, extra)
			p.SourceFile = "pages/p.md"
			got := ucpFindingsOf(t, ucpOn(), p)
			if len(got) != len(tc.want) {
				t.Errorf("%d finding(s), want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			joined := strings.Join(got, "\n")
			for _, w := range tc.want {
				if !strings.Contains(joined, w) {
					t.Errorf("no finding %q in\n%s", w, joined)
				}
			}
			for _, line := range got {
				if !strings.HasPrefix(line, "shop/pages/p.md | products.xml item ") && !strings.HasPrefix(line, "feeds: /products.xml | ucp:") {
					t.Errorf("finding does not name the source file and feed: %s", line)
				}
			}
		})
	}
}

// TestCheckUCPOptedOutFeed: a feed that turns checkout off for everything is
// not told that nothing is eligible.
func TestCheckUCPOptedOutFeed(t *testing.T) {
	if got := ucpFindingsOf(t, &models.UCPOptions{CheckoutEligibility: boolPtr(false)}, productPage("k", kettle, nil)); len(got) != 0 {
		t.Errorf("findings for a deliberate opt-out: %v", got)
	}
}

// TestCheckUCPModes: off, or no UCP feed, is silent; a problem fails strict;
// with the check off the build says once that there is something to see.
func TestCheckUCPModes(t *testing.T) {
	g, out := merchantGen(t, []models.Page{productPage("k", withOffer(kettle, "availability", "PreOrder"), nil)}, nil)
	if err := g.checkUCPIfRequested(); err != nil {
		t.Errorf("off: %v", err)
	}
	g.config.CheckUCP = "strict"
	if err := g.checkUCPIfRequested(); err != nil {
		t.Errorf("no UCP feed, nothing to check: %v", err)
	}
	g.config.CheckUCP, g.config.Quiet = "", false
	writeMerchant(t, g, out, models.FeedSpec{Path: "/products.xml", UCP: ucpOn()})
	g.config.CheckUCP = "strict"
	if err := g.checkUCPIfRequested(); err == nil || !strings.Contains(err.Error(), "UCP feed problem") {
		t.Errorf("strict with a restricted row: %v", err)
	}
	g.config.CheckUCP = "warn"
	if err := g.checkUCPIfRequested(); err != nil {
		t.Errorf("warn never fails: %v", err)
	}
	clean, cout := merchantGen(t, []models.Page{productPage("k", kettle, nil)}, nil)
	clean.config.CheckUCP, clean.config.Quiet = "strict", false
	writeMerchant(t, clean, cout, models.FeedSpec{Path: "/products.xml", UCP: ucpOn()})
	if err := clean.checkUCPIfRequested(); err != nil {
		t.Errorf("a clean feed passes strict: %v", err)
	}
}

// TestUCPFeedGoldenSite builds a small shop end to end and holds its Merchant
// feed, with `ucp:` and without, to golden files byte for byte. -update-ucp
// rewrites them.
func TestUCPFeedGoldenSite(t *testing.T) {
	out, err := buildUCPSite(t, "warn")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"products-ucp.xml", "products.xml"} {
		got := mustRead(t, filepath.Join(out, name))
		golden := filepath.Join("testdata", "ucp", name)
		if *updateUCPGolden {
			mustWrite(t, golden, got)
		}
		if want := mustRead(t, golden); got != want {
			t.Errorf("%s differs from %s (go test -run TestUCPFeedGoldenSite -update-ucp):\n%s", name, golden, got)
		}
	}
	if !strings.Contains(mustRead(t, filepath.Join(out, "products-ucp.xml")), "native_commerce") ||
		strings.Contains(mustRead(t, filepath.Join(out, "products.xml")), "native_commerce") {
		t.Error("only the ucp: feed carries native_commerce")
	}
	again, _ := buildUCPSite(t, "warn")
	if mustRead(t, filepath.Join(again, "products-ucp.xml")) != mustRead(t, filepath.Join(out, "products-ucp.xml")) {
		t.Error("a second build writes other bytes")
	}
	// The refurbished radio is restricted: a strict check fails the build.
	if _, err := buildUCPSite(t, "strict"); err == nil || !strings.Contains(err.Error(), "UCP feed problem") {
		t.Errorf("strict build: %v", err)
	}
}

// buildUCPSite builds the fixture shop with check_ucp at mode and returns the
// output directory.
func buildUCPSite(t *testing.T, mode string) (string, error) {
	t.Helper()
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "content", "shop", "pages")
	mustWrite(t, filepath.Join(tmp, "content", "shop", "metadata.json"), `{}`)
	entries, err := os.ReadDir(filepath.Join("testdata", "ucp", "pages"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		mustWrite(t, filepath.Join(pages, e.Name()), mustRead(t, filepath.Join("testdata", "ucp", "pages", e.Name())))
	}
	out := filepath.Join(tmp, "output")
	merchant := models.MerchantDefaults{Brand: "Example"}
	gen, err := New(Config{Source: "shop", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: out, Quiet: true, SEO: true, CheckUCP: mode,
		Feeds: []models.FeedSpec{
			{Path: "/products.xml", Format: merchantFormat, Type: "page", Merchant: merchant},
			{Path: "/products-ucp.xml", Format: merchantFormat, Type: "page", Merchant: merchant, UCP: ucpOn()},
		}})
	if err != nil {
		t.Fatal(err)
	}
	return out, gen.Generate()
}
