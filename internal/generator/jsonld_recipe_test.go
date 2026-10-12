package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestRecipeCategoryFromCategories: a Recipe takes recipeCategory from the
// post's categories unless its section or its own schema already set one.
func TestRecipeCategoryFromCategories(t *testing.T) {
	root := t.TempDir()
	g := sectionGen(root, map[string]map[string]interface{}{"posts": {"@type": "Recipe"}})
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Cakes"}, 2: {ID: 2, Name: "Desserts"}}
	post := func(cats []int, schema map[string]interface{}) models.Page {
		return models.Page{Title: "Cheesecake", Slug: "cheesecake", Type: "post", Categories: cats,
			SourceDir: filepath.Join(root, "food", "posts"), Schema: schema}
	}
	for _, tt := range []struct {
		page models.Page
		want string
	}{
		{post([]int{1}, nil), `"recipeCategory":"Cakes"`},
		{post([]int{1, 2, 1}, nil), `"recipeCategory":["Cakes","Desserts"]`},
		{post([]int{1}, map[string]interface{}{"recipeCategory": "Baking"}), `"recipeCategory":"Baking"`},
	} {
		if out := g.buildJSONLD(tt.page, true); !strings.Contains(out, tt.want) {
			t.Errorf("want %s in:\n%s", tt.want, out)
		}
	}
	if out := g.buildJSONLD(post(nil, nil), true); strings.Contains(out, "recipeCategory") {
		t.Errorf("no categories, no recipeCategory:\n%s", out)
	}
	article := post([]int{1}, nil)
	article.SourceDir = filepath.Join(root, "food", "news")
	if out := g.buildJSONLD(article, true); strings.Contains(out, "recipeCategory") {
		t.Errorf("only a Recipe gets one:\n%s", out)
	}
}

func TestHasLDType(t *testing.T) {
	for _, tt := range []struct {
		t    interface{}
		want bool
	}{
		{"Recipe", true}, {"HowTo", false},
		{[]interface{}{"HowTo", "Recipe"}, true}, {[]interface{}{"HowTo", 3}, false}, {nil, false},
	} {
		if got := hasLDType(map[string]interface{}{"@type": tt.t}, "Recipe"); got != tt.want {
			t.Errorf("hasLDType(%v) = %v", tt.t, got)
		}
	}
	if strings.Join(uniqueStrings([]string{"a", "", "b", "a"}), ",") != "a,b" {
		t.Error("uniqueStrings")
	}
}

func TestMissingRecommendedProps(t *testing.T) {
	got := missingRecommendedProps(`{"@type":"Recipe","description":"d","cookTime":"PT1H","recipeYield":"4"}`)
	if len(got) != 1 || got[0] != "Recipe could add nutrition, recipeCategory, recipeCuisine" {
		t.Errorf("got %v", got)
	}
	if got := missingRecommendedProps(`{"@type":"Recipe","totalTime":"PT1H","description":"d","nutrition":{"calories":"1"},` +
		`"recipeCategory":"c","recipeCuisine":"c","recipeYield":"1"}`); got != nil {
		t.Errorf("complete recipe: %v", got)
	}
	if missingRecommendedProps(`{bad`) != nil || missingRecommendedProps(`{"@type":"Thing"}`) != nil {
		t.Error("invalid JSON and unknown types say nothing")
	}
}

// TestRecommendedIsAdvice: a Recipe with every required property passes a
// strict check even when recommended ones are missing.
func TestRecommendedIsAdvice(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "r", "index.html"), `<html><head><script type="application/ld+json">`+
		`{"@context":"https://schema.org","@type":"Recipe","image":"x.jpg","recipeIngredient":["egg"],"recipeInstructions":["Bake."]}`+
		`</script></head><body></body></html>`)
	g := &Generator{config: Config{OutputDir: out, Quiet: true, CheckSchema: "strict", CheckSchemaRecommended: true}}
	if err := g.checkSchemaIfRequested(); err != nil {
		t.Fatalf("recommended properties must not fail strict: %v", err)
	}
}
