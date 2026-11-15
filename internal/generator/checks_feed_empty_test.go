package generator

// A declared product feed that comes out with no rows is a finding of
// check_acp and check_ucp, and a one-line notice with the checks off: it used
// to pass silently, typically because the product data sat in plain front
// matter (`sku:`, `price:`) that no feed reads.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// plainProductPage is a product described only in plain front matter — no
// Product JSON-LD, so no feed row.
func plainProductPage() models.Page {
	return models.Page{Title: "Book", Slug: "book", Type: "page", Description: "A book", SourceDir: "shop",
		Extra: map[string]interface{}{"sku": "BOOK-1", "price": "24.00"}}
}

// noImage is a Product the feeds leave out: it has no image.
var noImage = map[string]interface{}{"@type": "Product", "name": "Ghost", "sku": "GHOST", "brand": "B",
	"offers": map[string]interface{}{"price": "3.00", "priceCurrency": "EUR", "availability": "InStock"}}

func TestEmptyFeedFinding(t *testing.T) {
	if _, empty := emptyFeedFinding("a.jsonl", "/a.jsonl", 2, 1); empty {
		t.Error("a feed with a row is not empty")
	}
	f, empty := emptyFeedFinding("a.jsonl", "/a.jsonl", 0, 0)
	if !empty || f.file != "feeds: /a.jsonl" || !strings.Contains(f.detail, `a.jsonl has no rows — no selected page has Product JSON-LD; give each product page schema: with "@type": Product, a sku`) {
		t.Errorf("no products: %+v", f)
	}
	if f, _ := emptyFeedFinding("a.jsonl", "/a.jsonl", 3, 0); !strings.Contains(f.detail, "all 3 product(s) are left out for a missing required field") {
		t.Errorf("every product dropped: %+v", f)
	}
}

// TestCheckACPEmptyFeed: warn names the feed and the cause, strict fails,
// and with the check off the build prints one line.
func TestCheckACPEmptyFeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages []models.Page
		want  string
	}{
		{"plain front matter", []models.Page{plainProductPage()}, "no selected page has Product JSON-LD"},
		{"every row dropped", []models.Page{productPage("ghost", noImage, nil)}, "all 1 product(s) are left out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := acpCheckGen(t, tc.pages)
			g.config.Quiet = false
			printed, err := capture(t, func() error {
				return g.writeDeclaredFeed(models.FeedSpec{Path: "/acp.jsonl", Format: acpFormat, ACP: acpSeller()})
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(printed, "acp.jsonl has no rows — "+tc.want) || !strings.Contains(printed, "set check_acp: warn") {
				t.Errorf("check off: no notice in\n%s", printed)
			}
			g.config.CheckACP = "warn"
			printed, err = capture(t, g.checkACPIfRequested)
			if err != nil || !strings.Contains(printed, "ACP feed in feeds: /acp.jsonl → acp.jsonl has no rows — "+tc.want) {
				t.Errorf("warn: %v\n%s", err, printed)
			}
			g.config.CheckACP = "strict"
			if _, err := capture(t, g.checkACPIfRequested); err == nil || !strings.Contains(err.Error(), "ACP feed problem") {
				t.Errorf("strict fails on an empty feed: %v", err)
			}
		})
	}
}

// TestACPEmptyFeedQuietOrChecked: no notice when quiet, nor when the check
// itself will report it.
func TestACPEmptyFeedQuietOrChecked(t *testing.T) {
	g, _ := acpCheckGen(t, []models.Page{plainProductPage()})
	printed, _ := capture(t, func() error {
		return g.writeDeclaredFeed(models.FeedSpec{Path: "/acp.jsonl", Format: acpFormat, ACP: acpSeller()})
	})
	if strings.Contains(printed, "has no rows") {
		t.Errorf("quiet build printed:\n%s", printed)
	}
	g.config.Quiet, g.config.CheckACP = false, "warn"
	printed, _ = capture(t, func() error {
		return g.writeDeclaredFeed(models.FeedSpec{Path: "/b.jsonl", Format: acpFormat, ACP: acpSeller()})
	})
	if strings.Contains(printed, "set check_acp: warn") {
		t.Errorf("check on, the notice is the check's:\n%s", printed)
	}
}

// TestCheckUCPEmptyFeed: the same for a Merchant feed with `ucp:`.
func TestCheckUCPEmptyFeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages []models.Page
		want  string
	}{
		{"plain front matter", []models.Page{plainProductPage()}, "no selected page has Product JSON-LD"},
		{"every row dropped", []models.Page{productPage("ghost", noImage, nil)}, "all 1 product(s) are left out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, out := merchantGen(t, tc.pages, nil)
			g.config.Quiet = false
			printed, _ := capture(t, func() error {
				writeMerchant(t, g, out, models.FeedSpec{Path: "/products.xml", UCP: ucpOn()})
				return nil
			})
			if !strings.Contains(printed, "products.xml has no rows — "+tc.want) || !strings.Contains(printed, "set check_ucp: warn") {
				t.Errorf("check off: no notice in\n%s", printed)
			}
			got := ucpFindings(g.ucpFeeds[0])
			last := got[len(got)-1]
			if last.file != "feeds: /products.xml" || !strings.Contains(last.detail, tc.want) {
				t.Errorf("findings: %+v", got)
			}
			for _, f := range got {
				if strings.Contains(f.detail, "no row is eligible") {
					t.Errorf("an empty feed is not also told nothing is eligible: %+v", got)
				}
			}
			g.config.CheckUCP = "strict"
			if _, err := capture(t, g.checkUCPIfRequested); err == nil || !strings.Contains(err.Error(), "UCP feed problem") {
				t.Errorf("strict fails on an empty feed: %v", err)
			}
		})
	}
}
