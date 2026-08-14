package models

// Navigation from the source CMS (#132). A menu is an editorial arrangement:
// nothing in the content records which entry came first or what it was called,
// so the tree has to survive the export exactly as the site rendered it.

import (
	"testing"
)

func item(id, parent, order int, title string) MenuItem {
	return MenuItem{ID: id, Parent: parent, Order: order, Title: title, URL: "/" + title}
}

// TestMenuTreeNestsAndOrders: children hang off their parent, every level is
// in menu order, and grandchildren survive — the flat list an export delivers
// is the CMS's storage, not the shape a template walks.
func TestMenuTreeNestsAndOrders(t *testing.T) {
	m := Menu{Items: []MenuItem{
		item(3, 1, 2, "services-b"),
		item(1, 0, 2, "services"),
		item(2, 0, 1, "about"),
		item(4, 1, 1, "services-a"),
		item(5, 4, 1, "deep"),
	}}
	tree := m.Tree()
	if len(tree) != 2 {
		t.Fatalf("top level = %d entries, want 2", len(tree))
	}
	if tree[0].Title != "about" || tree[1].Title != "services" {
		t.Fatalf("top level out of order: %s, %s", tree[0].Title, tree[1].Title)
	}
	kids := tree[1].Children
	if len(kids) != 2 || kids[0].Title != "services-a" || kids[1].Title != "services-b" {
		t.Fatalf("children wrong: %+v", kids)
	}
	if len(kids[0].Children) != 1 || kids[0].Children[0].Title != "deep" {
		t.Fatalf("grandchild lost: %+v", kids[0].Children)
	}
}

// TestMenuTreeKeepsOrphans: an item whose parent is missing from the menu is
// promoted rather than dropped — a half-exported menu still renders its links.
func TestMenuTreeKeepsOrphans(t *testing.T) {
	m := Menu{Items: []MenuItem{item(1, 99, 1, "orphan"), item(2, 0, 2, "root")}}
	tree := m.Tree()
	if len(tree) != 2 || tree[0].Title != "orphan" {
		t.Fatalf("orphan must survive at the top: %+v", tree)
	}
}

// TestMenuTreeSurvivesCycles: a tangled parent chain must not hide entries or
// hang the build.
func TestMenuTreeSurvivesCycles(t *testing.T) {
	m := Menu{Items: []MenuItem{item(1, 2, 1, "a"), item(2, 1, 2, "b"), item(3, 0, 3, "c")}}
	tree := m.Tree()
	if len(tree) != 3 {
		t.Fatalf("a cycle must not swallow entries: %+v", tree)
	}
	// Self-parenting is the same case.
	self := Menu{Items: []MenuItem{item(7, 7, 1, "self")}}
	if got := self.Tree(); len(got) != 1 || got[0].Title != "self" {
		t.Fatalf("self-parented item lost: %+v", got)
	}
}

func TestMenuTreeEmpty(t *testing.T) {
	if got := (Menu{}).Tree(); got != nil {
		t.Fatalf("an empty menu is nil, got %+v", got)
	}
}

// TestMenusByLocation: a template asks for "primary"; a menu the theme never
// assigned is still reachable by its slug.
func TestMenusByLocation(t *testing.T) {
	menus := []Menu{
		{Name: "Main", Slug: "main", Locations: []string{"primary", "mobile"}},
		{Name: "Legal", Slug: "legal"},
		{Name: "Second", Slug: "second", Locations: []string{"primary"}},
		{Name: "Unnamed"},
	}
	byLoc := MenusByLocation(menus)
	if byLoc["primary"].Name != "Main" {
		t.Fatalf("first menu must keep a contested location, got %q", byLoc["primary"].Name)
	}
	if byLoc["mobile"].Name != "Main" || byLoc["main"].Name != "Main" {
		t.Fatal("a menu is reachable by every location and by its slug")
	}
	if byLoc["legal"].Name != "Legal" {
		t.Fatal("an unassigned menu must still be reachable by slug")
	}
	if _, ok := byLoc[""]; ok {
		t.Fatal("a blank key must never be registered")
	}
	if got := MenusByLocation(nil); len(got) != 0 {
		t.Fatalf("no menus → empty map, got %v", got)
	}
}
