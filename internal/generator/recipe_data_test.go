package generator

import (
	"reflect"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestRecipeOf: every shape recipeInstructions takes flattens to step texts,
// times are written for people, and a node that is not a Recipe gives none.
func TestRecipeOf(t *testing.T) {
	ld := map[string]interface{}{
		"@type": []interface{}{"Recipe", "HowTo"}, "name": "  Pierogi <em>ruskie</em> ",
		"recipeYield": []interface{}{"4 servings", 24.0},
		"prepTime":    "PT30M", "cookTime": "about an hour", "totalTime": "",
		"recipeIngredient": []string{"500 g flour", " ", "1 egg &amp; water"},
		"recipeInstructions": []interface{}{
			"Make the dough.\n\nRest it.",
			map[string]interface{}{"@type": "HowToStep", "text": "<p>Roll it out.</p>"},
			map[string]interface{}{"@type": "HowToStep", "name": "Fill"},
			map[string]interface{}{"@type": "HowToStep"},
			map[string]interface{}{"@type": "HowToSection", "name": "Cooking", "itemListElement": []interface{}{
				map[string]interface{}{"@type": "HowToStep", "text": "Boil."}, "Serve."}},
			42,
		},
	}
	r, ok := recipeOf(ld, "en")
	if !ok || !r.complete() {
		t.Fatalf("recipe = %+v, %v", r, ok)
	}
	want := recipeData{
		Name: "Pierogi ruskie", Yield: "4 servings, 24",
		Times:       []recipeTime{{"prep", "30 min"}, {"cook", "about an hour"}},
		Ingredients: []string{"500 g flour", "1 egg & water"},
		Steps:       []string{"Make the dough.", "Rest it.", "Roll it out.", "Fill", "Boil.", "Serve."},
	}
	if !reflect.DeepEqual(r, want) {
		t.Errorf("recipe =\n%+v\nwant\n%+v", r, want)
	}
	if _, ok := recipeOf(map[string]interface{}{"@type": "BlogPosting"}, "en"); ok {
		t.Error("a BlogPosting is not a recipe")
	}
	one, _ := recipeOf(map[string]interface{}{"@type": "Recipe", "recipeYield": 4, "recipeIngredient": "salt"}, "en")
	if one.Yield != "4" || len(one.Ingredients) != 1 || one.complete() {
		t.Errorf("single values = %+v", one)
	}
	if ldText(true) != "" || ldList(nil) != nil {
		t.Error("only strings and numbers are text")
	}
}

// TestPageRecipe reads the recipe from the page's merged schema.
func TestPageRecipe(t *testing.T) {
	g := sectionGen(t.TempDir(), nil)
	page := models.Page{Title: "Soup", Slug: "soup", Content: "<p>x</p>", Schema: map[string]interface{}{
		"@type": "Recipe", "recipeIngredient": []interface{}{"water"}, "recipeInstructions": []interface{}{"Boil."}}}
	r, ok := g.pageRecipe(page, true)
	if !ok || r.Name != "Soup" || !r.complete() {
		t.Errorf("page recipe = %+v %v", r, ok)
	}
	page.Lang = "pl"
	page.Schema["totalTime"] = "PT1H"
	if r, _ := g.pageRecipe(page, true); len(r.Times) != 1 || r.Times[0].Value == "" {
		t.Errorf("times = %+v", r.Times)
	}
}
