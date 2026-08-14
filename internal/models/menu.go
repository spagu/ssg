package models

// Navigation, as the source site defined it (#132). A menu is the one thing a
// migration cannot reconstruct from the content: nothing in a page records
// which menu it belonged to, in what order, or under what label — an editor
// arranged it by hand, and that arrangement lives only in the CMS.
//
// WordPress gates menus behind `edit_theme_options`, so an export without
// credentials comes back with none; the migration says so rather than letting
// a site come up silently unnavigable.

import "sort"

// Menu is one navigation menu with its items in the order the site renders.
type Menu struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Locations are the theme slots the menu was assigned to ("primary",
	// "footer"). A menu can hold none — it is then reachable by its own slug.
	Locations []string   `json:"locations"`
	Items     []MenuItem `json:"items"`
}

// MenuItem is one entry. Parent/Order describe the tree as the CMS stored it;
// Children is the resolved form templates walk.
type MenuItem struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Parent int    `json:"parent"`
	Order  int    `json:"order"`
	// Type/Object/ObjectID say what the item points at ("post_type"/"page",
	// "taxonomy"/"category", "custom"), which a theme can use to mark the
	// current section.
	Type     string `json:"type"`
	Object   string `json:"object"`
	ObjectID int    `json:"object_id"`

	Children []MenuItem `json:"-"`
}

// Tree returns the menu's items as a hierarchy: top-level entries in menu
// order, each carrying its children. The flat list an export delivers is what
// the CMS stored; a template needs the shape a reader sees.
//
// An item whose parent is missing from the menu is promoted to the top rather
// than dropped — a half-exported menu still renders every link it has.
func (m Menu) Tree() []MenuItem {
	nodes := make(map[int]*menuNode, len(m.Items))
	flat := make([]*menuNode, 0, len(m.Items))
	for _, item := range m.Items {
		item.Children = nil
		n := &menuNode{item: item}
		nodes[item.ID] = n
		flat = append(flat, n)
	}

	var roots []*menuNode
	for _, n := range flat {
		parent, ok := nodes[n.item.Parent]
		// No parent, a parent outside this menu, itself, or a parent chain
		// that loops: the item becomes a top-level entry instead of vanishing.
		if n.item.Parent == 0 || !ok || parent == n || cyclicParent(n, nodes) {
			roots = append(roots, n)
			continue
		}
		parent.children = append(parent.children, n)
	}
	return materializeMenu(roots)
}

// menuNode builds the tree with pointers so a child's own children are already
// attached by the time the level above is materialised.
type menuNode struct {
	item     MenuItem
	children []*menuNode
}

// cyclicParent reports whether following an item's parents loops instead of
// reaching the top — a menu an editor (or a broken export) tangled.
func cyclicParent(start *menuNode, nodes map[int]*menuNode) bool {
	seen := map[int]bool{start.item.ID: true}
	for cur := start; ; {
		parent, ok := nodes[cur.item.Parent]
		if !ok || parent.item.Parent == 0 {
			return false
		}
		if seen[parent.item.ID] {
			return true
		}
		seen[parent.item.ID] = true
		cur = parent
	}
}

// materializeMenu turns a level of nodes into items, ordered by the CMS's
// menu_order, with each item's children resolved first.
func materializeMenu(nodes []*menuNode) []MenuItem {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]MenuItem, 0, len(nodes))
	for _, n := range nodes {
		item := n.item
		item.Children = materializeMenu(n.children)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// MenusByLocation keys menus for template lookup: by every theme location they
// occupy, and by their own slug, so a menu the theme never assigned is still
// reachable. A location claimed twice keeps the first menu — the export lists
// them in the site's own order, and a template asking for "primary" wants one
// menu, not a surprise.
func MenusByLocation(menus []Menu) map[string]Menu {
	out := make(map[string]Menu, len(menus)*2)
	for _, m := range menus {
		for _, loc := range m.Locations {
			if loc == "" {
				continue
			}
			if _, taken := out[loc]; !taken {
				out[loc] = m
			}
		}
	}
	for _, m := range menus {
		if m.Slug == "" {
			continue
		}
		if _, taken := out[m.Slug]; !taken {
			out[m.Slug] = m
		}
	}
	return out
}
