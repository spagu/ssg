package generator

// check_acp (GO-162) and a whole fixture site's ACP feed against a golden
// file (GO-161).

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

var updateACPGolden = flag.Bool("update-acp", false, "rewrite testdata/acp/feed.jsonl")

// acpCheckGen is a generator with an output tree where /mug/ and /privacy/
// exist and nothing else does.
func acpCheckGen(t *testing.T, pages []models.Page) (*Generator, string) {
	t.Helper()
	g, out := merchantGen(t, pages, nil)
	mustWrite(t, filepath.Join(out, "index.html"), "<p>home</p>")
	mustWrite(t, filepath.Join(out, "mug", "index.html"), "<p>mug</p>")
	mustWrite(t, filepath.Join(out, "privacy", "index.html"), "<p>privacy</p>")
	return g, out
}

// TestCheckACPFindings: every finding, each naming the source file and the
// field.
func TestCheckACPFindings(t *testing.T) {
	bare := map[string]interface{}{"@type": "Product", "name": "Bare", "sku": "BARE", "image": "/b.jpg", "brand": "B",
		"offers": map[string]interface{}{"price": "3", "availability": "InStock"}}
	dupe := map[string]interface{}{"@type": "Product", "name": "Mug again", "sku": "MUG-1", "image": "/m.jpg",
		"offers": map[string]interface{}{"price": "12,50", "priceCurrency": "PLN", "sku": "OFFER-1"}}
	other := map[string]interface{}{"@type": "Product", "name": "Cup", "sku": "CUP", "image": "/c.jpg", "brand": "C",
		"offers": map[string]interface{}{"price": "9.00", "priceCurrency": "EUR", "sku": "OFFER-1", "availability": "InStock"}}
	pages := []models.Page{
		productPage("mug", mug, map[string]interface{}{"acp": map[string]interface{}{
			"is_eligible_checkout": true, "is_eligible_search": false, "sale_price": "20 PLN", "shipping_price": "5.00 EUR"}}),
		productPage("bare", bare, nil), productPage("dupe", dupe, nil),
		productPage("cup", other, map[string]interface{}{"acp": map[string]interface{}{"sale_price": "cheap"}}),
	}
	pages[0].SourceDir, pages[0].SourceFile = "content/shop/pages", "mug.md"
	g, _ := acpCheckGen(t, pages)
	seller := models.ACPSeller{SellerName: "S", TargetCountries: []string{"PL"}, StoreCountry: "PL",
		SellerURL: "https://elsewhere.example.org/", ReturnPolicy: "/returns/"}
	if err := g.writeDeclaredFeed(models.FeedSpec{Path: "/acp.jsonl", Format: acpFormat, ACP: seller}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range g.acpFindings(g.acpFeeds[0]) {
		got = append(got, f.file+" | "+f.detail)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		"content/shop/pages/mug.md | acp.jsonl item MUG-1: sale_price — 20 PLN must be above zero and below the price, 12.5 PLN",
		"content/shop/pages/mug.md | acp.jsonl item MUG-1: shipping_price — 5.00 EUR is not in the price's currency, PLN",
		"content/shop/pages/mug.md | acp.jsonl item MUG-1: is_eligible_checkout — true while is_eligible_search is false",
		"/bare/ | acp.jsonl item BARE: price — 3 has no currency; set offers.priceCurrency",
		"/bare/ | acp.jsonl item BARE: url — https://example.com/bare/ does not resolve in the output",
		"/dupe/ | acp.jsonl item MUG-1: item_id — also used by content/shop/pages/mug.md",
		"/dupe/ | acp.jsonl item MUG-1: brand — required and missing",
		"/dupe/ | acp.jsonl item MUG-1: price — 12,50 PLN is not \"amount CURRENCY\"",
		"/dupe/ | acp.jsonl item MUG-1: availability — in_stock is stated by no page",
		"/dupe/ | acp.jsonl item MUG-1: offer_id — also used by /cup/",
		"/cup/ | acp.jsonl item CUP: sale_price — cheap is not \"amount CURRENCY\"",
		"feeds: /acp.jsonl | acp.seller_privacy_policy — required when a row has is_eligible_checkout: true",
		"feeds: /acp.jsonl | acp.seller_tos — required when a row has is_eligible_checkout: true",
		"feeds: /acp.jsonl | acp.return_policy — https://example.com/returns/ does not resolve in the output",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing finding %q in:\n%s", want, all)
		}
	}
	if strings.Contains(all, "BARE: price — required") || strings.Contains(all, "seller_url") ||
		strings.Contains(all, "url — https://example.com/mug/") {
		t.Errorf("one finding per cause; another site's URL and a resolving one pass:\n%s", all)
	}
	g.config.CheckACP = "strict"
	if err := g.checkACPIfRequested(); err == nil || !strings.Contains(err.Error(), "ACP feed problem") {
		t.Errorf("strict fails the build: %v", err)
	}
	g.config.CheckACP = "warn"
	if err := g.checkACPIfRequested(); err != nil {
		t.Errorf("warn only reports: %v", err)
	}
}

// TestCheckACPModes: off, or no ACP feed, is silent; a clean feed passes.
func TestCheckACPModes(t *testing.T) {
	g, _ := acpCheckGen(t, []models.Page{productPage("mug", mug, nil)})
	if err := g.checkACPIfRequested(); err != nil {
		t.Errorf("off: %v", err)
	}
	g.config.Strict = true
	if err := g.checkACPIfRequested(); err != nil {
		t.Errorf("no ACP feed, nothing to check: %v", err)
	}
	seller := acpSeller()
	seller.ReturnPolicy, seller.SellerTOS = "", "https://elsewhere.example.org/terms"
	if err := g.writeDeclaredFeed(models.FeedSpec{Path: "/acp.jsonl", Format: acpFormat, ACP: seller}); err != nil {
		t.Fatal(err)
	}
	g.config.Quiet = false
	if err := g.checkACPIfRequested(); err != nil {
		t.Errorf("a clean feed passes strict: %v", err)
	}
	g.config.BasePath = "/shop"
	if g.acpURLResolves("/outside/") {
		t.Error("a path outside the base path is not this site's")
	}
}

func TestACPMoneyProblem(t *testing.T) {
	for _, tc := range []struct {
		value, price string
		sale         bool
		want         string
	}{
		{"5.00 EUR", "10 EUR", false, ""},
		{"5.00 EUR", "10 EUR", true, ""},
		{"0 EUR", "10 EUR", true, "above zero"},
		{"5 eur", "10 EUR", false, "amount CURRENCY"},
		{"5 USD", "10 EUR", false, "currency, EUR"},
		{"5 USD", "", false, ""},
	} {
		got := acpMoneyProblem(tc.value, tc.price, tc.sale)
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("%q vs %q (sale %v) = %q, want %q", tc.value, tc.price, tc.sale, got, tc.want)
		}
	}
}

// TestACPFeedGoldenSite builds a small shop end to end — Markdown with
// schema: front matter, a ProductGroup, merchant: and acp: keys — and holds
// the feed to testdata/acp/feed.jsonl byte for byte. -update-acp rewrites it.
func TestACPFeedGoldenSite(t *testing.T) {
	out, err := buildACPSite(t, "warn", "")
	if err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, filepath.Join(out, "acp.jsonl"))
	golden := filepath.Join("testdata", "acp", "feed.jsonl")
	if *updateACPGolden {
		mustWrite(t, golden, got)
	}
	if want := mustRead(t, golden); got != want {
		t.Errorf("feed differs from %s (go test -run TestACPFeedGoldenSite -update-acp):\n%s", golden, got)
	}
	if again, _ := buildACPSite(t, "warn", ""); mustRead(t, filepath.Join(again, "acp.jsonl")) != got {
		t.Error("a second build writes other bytes")
	}
	// A product with no price fails a strict check, and with it the build.
	if _, err := buildACPSite(t, "strict", "---\ntitle: Ghost\nslug: ghost\nstatus: publish\ntype: page\n"+
		"schema:\n  \"@type\": Product\n  sku: GHOST\n---\n\nNo offer.\n"); err == nil || !strings.Contains(err.Error(), "ACP feed problem") {
		t.Errorf("strict build: %v", err)
	}
}

// buildACPSite builds the fixture shop with check_acp at mode, plus one more
// page when extra is set, and returns the output directory.
func buildACPSite(t *testing.T, mode, extra string) (string, error) {
	t.Helper()
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "content", "shop", "pages")
	mustWrite(t, filepath.Join(tmp, "content", "shop", "metadata.json"), `{}`)
	entries, err := os.ReadDir(filepath.Join("testdata", "acp", "pages"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		mustWrite(t, filepath.Join(pages, e.Name()), mustRead(t, filepath.Join("testdata", "acp", "pages", e.Name())))
	}
	if extra != "" {
		mustWrite(t, filepath.Join(pages, "ghost.md"), extra)
	}
	yes := true
	seller := models.ACPSeller{SellerName: "Example Shop", SellerURL: "/", SellerPrivacyPolicy: "/privacy/",
		SellerTOS: "/terms/", TargetCountries: []string{"PL", "DE"}, StoreCountry: "PL", IsEligibleCheckout: &yes}
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "shop", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: out, Quiet: true, SEO: true, CheckACP: mode,
		Feeds: []models.FeedSpec{{Path: "/acp.jsonl", Format: acpFormat, Type: "page", ACP: seller,
			Merchant: models.MerchantDefaults{Brand: "Example"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return out, gen.Generate()
}
