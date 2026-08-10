package generator

// #111: validating the structured data a page actually emits.

import (
	"strings"
	"testing"
)

// TestMissingSchemaPropsReportsRequiredOnes is the reported case: both of these
// build cleanly today and are rejected by search engines with nothing the author
// can see.
func TestMissingSchemaPropsReportsRequiredOnes(t *testing.T) {
	got := missingSchemaProps(`{"@type":"Recipe","cookTime":"PT20M"}`)
	if len(got) != 1 || !strings.Contains(got[0], "image") ||
		!strings.Contains(got[0], "recipeIngredient") {
		t.Errorf("recipe: got %v", got)
	}

	got = missingSchemaProps(`{"@type":"Product","name":"X","image":"/a.jpg",
		"offers":{"@type":"Offer","price":"1899.00"}}`)
	if len(got) != 1 || !strings.Contains(got[0], "Offer is missing priceCurrency") {
		t.Errorf("nested offer: got %v", got)
	}
}

// TestMissingSchemaPropsAcceptsCompleteData: the check must not cry wolf, or it
// trains people to ignore it.
func TestMissingSchemaPropsAcceptsCompleteData(t *testing.T) {
	complete := `{"@type":"Recipe","name":"Pierogi","image":"/p.jpg",
		"recipeIngredient":["flour"],"recipeInstructions":[{"@type":"HowToStep","text":"Boil"}]}`
	if got := missingSchemaProps(complete); len(got) != 0 {
		t.Errorf("complete recipe reported: %v", got)
	}
}

// TestMissingSchemaPropsIgnoresUnknownTypes keeps the generality the feature
// exists for: schema.org has hundreds of types, and an unknown one passing
// silently is what lets any of them be used at all.
func TestMissingSchemaPropsIgnoresUnknownTypes(t *testing.T) {
	for _, in := range []string{
		`{"@type":"Car","brand":"Skoda"}`,
		`{"@type":"SomethingNobodyHasHeardOf"}`,
		`{"@type":"WebPage","name":"x"}`,
	} {
		if got := missingSchemaProps(in); len(got) != 0 {
			t.Errorf("%s reported: %v", in, got)
		}
	}
}

// TestMissingSchemaPropsReportsBrokenJSON: a block a crawler cannot parse is
// worse than no block, and it is invisible in the rendered page.
func TestMissingSchemaPropsReportsBrokenJSON(t *testing.T) {
	got := missingSchemaProps(`{"@type":"Recipe",`)
	if len(got) != 1 || !strings.Contains(got[0], "not valid JSON") {
		t.Errorf("got %v", got)
	}
}

// TestHasLDPropTreatsEmptyAsAbsent: an empty string or list is the same as
// missing to a consumer, so accepting it would let the check pass on data that
// still gets rejected.
func TestHasLDPropTreatsEmptyAsAbsent(t *testing.T) {
	for name, obj := range map[string]map[string]interface{}{
		"empty string": {"image": ""},
		"blank string": {"image": "   "},
		"empty list":   {"image": []interface{}{}},
		"empty object": {"image": map[string]interface{}{}},
		"null":         {"image": nil},
	} {
		if hasLDProp(obj, "image") {
			t.Errorf("%s counted as present", name)
		}
	}
	if !hasLDProp(map[string]interface{}{"image": "/a.jpg"}, "image") {
		t.Error("a real value was not counted")
	}
	if !hasLDProp(map[string]interface{}{"seatingCapacity": 5}, "seatingCapacity") {
		t.Error("a number was not counted")
	}
}

// TestWalkLDFindsNestedObjects: an Offer inside a Product is the case that
// matters, since a missing priceCurrency there invalidates the Product too.
func TestWalkLDFindsNestedObjects(t *testing.T) {
	doc := map[string]interface{}{
		"@type": "Product",
		"offers": map[string]interface{}{
			"@type":  "Offer",
			"seller": map[string]interface{}{"@type": "Organization"},
		},
		"list": []interface{}{map[string]interface{}{"@type": "Event"}},
	}
	var seen []string
	walkLD(doc, func(o map[string]interface{}) {
		if t, ok := o["@type"].(string); ok {
			seen = append(seen, t)
		}
	})
	for _, want := range []string{"Product", "Offer", "Organization", "Event"} {
		found := false
		for _, s := range seen {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("walkLD missed %s (saw %v)", want, seen)
		}
	}
}

// TestCheckSchemaOffByDefault: a validator that runs unasked is a behaviour
// change for every existing site.
func TestCheckSchemaOffByDefault(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir(), Quiet: true}}
	if err := g.checkSchemaIfRequested(); err != nil {
		t.Fatalf("off should be a no-op: %v", err)
	}
}
