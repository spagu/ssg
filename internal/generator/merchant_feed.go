package generator

// A Google Merchant Center product feed (#196).
//
// Google pulls the file on a schedule, so a product catalogue is one more
// static artefact: no credentials, no daemon, no push. The rows are a
// serialisation of what the pages already say about themselves — the merged
// JSON-LD every page gets — with `merchant:` front matter and the feed's own
// defaults for what Schema.org has no word for. A page that ships valid
// Product + Offer data needs no extra authoring to be listed.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// merchantFormat is the `format:` that asks for a product feed.
const merchantFormat = "google-merchant"

// merchantItem is one row of the feed.
type merchantItem struct {
	id, groupID, title, description, link, image     string
	price, availability, condition, brand, gtin, mpn string
	// page is the page's URL, for warnings; urlID marks an id that fell back
	// to it.
	page  string
	urlID bool
	// stockUnstated marks availability nobody gave, assumed in_stock.
	stockUnstated bool
}

// writeMerchantFeed writes the products of the spec's pages as a Merchant
// Center RSS 2.0 feed. Every product is listed: a catalogue is not a
// newest-first slice, so feed_items does not cap it unless the spec says so.
func (g *Generator) writeMerchantFeed(spec models.FeedSpec, rel string) error {
	var items []merchantItem
	for _, p := range g.merchantPages(spec) {
		items = append(items, g.merchantItemsOf(p, spec.Merchant)...)
	}
	items = g.keepAcceptedItems(rel, items)
	sort.SliceStable(items, func(i, j int) bool { return items[i].id < items[j].id })
	if spec.Items != nil && *spec.Items > 0 && len(items) > *spec.Items {
		items = items[:*spec.Items]
	}
	title := firstNonEmpty(strings.TrimSpace(spec.Title), g.config.Domain)
	return g.writeFeedFile(rel, g.renderMerchantFeed(title, items), len(items))
}

// merchantPages is the feed's selection. With no `type:` both posts and pages
// are searched: a shop's products are usually pages, a blog's reviews posts.
func (g *Generator) merchantPages(spec models.FeedSpec) []merchantPage {
	var out []merchantPage
	for _, kind := range []string{"post", "page"} {
		if spec.Type != "" && !strings.EqualFold(strings.TrimSpace(spec.Type), kind) {
			continue
		}
		s := spec
		s.Type = kind
		for _, p := range g.selectFeedPosts(s) {
			out = append(out, merchantPage{page: p, isPost: kind == "post"})
		}
	}
	return out
}

// merchantPage is a selected page and whether it renders as a post.
type merchantPage struct {
	page   models.Page
	isPost bool
}

// merchantItemsOf turns the products on one page into rows: one per Product,
// one per variant of a ProductGroup, with the page's `merchant:` front matter
// over the JSON-LD and the feed's defaults under it.
func (g *Generator) merchantItemsOf(mp merchantPage, defaults models.MerchantDefaults) []merchantItem {
	merged := g.mergedSchema(mp.page, mp.isPost)
	front := ldMap(mp.page.Extra["merchant"])
	pageURL := mp.page.GetURL()
	var out []merchantItem
	for _, node := range ldNodes(merged) {
		switch {
		case ldHasType(node, "ProductGroup"):
			groupID := firstNonEmpty(ldText(node["productGroupID"]), ldText(node["sku"]), pageURL)
			for _, v := range ldList(node["hasVariant"]) {
				item := g.productItem(inheritLD(ldMap(v), node), mp.page, defaults)
				item.groupID = groupID
				out = append(out, item)
			}
		case ldHasType(node, "Product"):
			out = append(out, g.productItem(node, mp.page, defaults))
		}
	}
	for i := range out {
		out[i].page = pageURL
		applyMerchantFront(&out[i], front, len(out) == 1)
		if out[i].id == "" {
			out[i].id, out[i].urlID = pageURL, true
		}
	}
	return out
}

// applyMerchantFront lays the page's `merchant:` keys over a row. id applies
// only to a page with one product: on variants it would give them all one id.
func applyMerchantFront(item *merchantItem, front map[string]interface{}, single bool) {
	if single && item.id == "" {
		item.id = ldText(front["id"])
	}
	for key, field := range map[string]*string{
		"availability": &item.availability, "condition": &item.condition,
		"brand": &item.brand, "gtin": &item.gtin, "mpn": &item.mpn,
	} {
		if v := ldText(front[key]); v != "" {
			*field = v
		}
	}
	item.stockUnstated = item.availability == ""
	item.availability = merchantAvailability(item.availability)
	item.condition = merchantCondition(item.condition)
}

// keepAcceptedItems drops the rows Google would reject and says why, and
// warns once about ids taken from the URL, which a renamed slug would change.
func (g *Generator) keepAcceptedItems(rel string, items []merchantItem) []merchantItem {
	out := items[:0]
	urlIDs, unstated := 0, 0
	for _, it := range items {
		if missing := it.missingRequired(); len(missing) > 0 {
			fmt.Printf("   ⚠️  %s: %s left out, Google rejects a product without %s\n", rel, it.page, strings.Join(missing, ", "))
			continue
		}
		if it.urlID {
			urlIDs++
		}
		if it.stockUnstated {
			unstated++
		}
		out = append(out, it)
	}
	if urlIDs > 0 {
		fmt.Printf("   ⚠️  %s: %d product(s) use their URL as id — a changed slug makes them new products; set sku or merchant.id\n", rel, urlIDs)
	}
	if unstated > 0 {
		fmt.Printf("   ⚠️  %s: %d product(s) state no availability and are listed in_stock; set offers.availability, merchant.availability or the feed's merchant.availability\n", rel, unstated)
	}
	return out
}

// missingRequired names the attributes Google rejects an item without.
func (it merchantItem) missingRequired() []string {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"title", it.title}, {"description", it.description}, {"link", it.link},
		{"image_link", it.image}, {"price", it.price},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	return missing
}
