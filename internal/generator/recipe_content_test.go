package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestRecipeSection(t *testing.T) {
	md := `Intro.

## Ingredients for ~16 servings

- 5 large **eggs**
- 200 g [sugar](/sugar/)

## Method

1. Set the oven to 160°C.
2. Whisk the eggs.

### Tips

Keep it cold.

## Notes

Done.
`
	if got := strings.Join(recipeSection(md, recipeHeadings["ingredients"]), "|"); got != "5 large eggs|200 g sugar" {
		t.Errorf("ingredients = %q", got)
	}
	if got := strings.Join(recipeSection(md, recipeHeadings["instructions"]), "|"); got != "Set the oven to 160°C.|Whisk the eggs." {
		t.Errorf("steps = %q", got)
	}
	bold := "**Składniki:**\n\n* mąka\n* jajka\n\n**Przygotowanie**\n\nUtrzeć jajka.\nDodać mąkę.\n\nPiec 40 minut.\n\n**Smacznego**\n"
	if got := strings.Join(recipeSection(bold, recipeHeadings["ingredients"]), "|"); got != "mąka|jajka" {
		t.Errorf("bold ingredients = %q", got)
	}
	if got := strings.Join(recipeSection(bold, recipeHeadings["instructions"]), "|"); got != "Utrzeć jajka. Dodać mąkę.|Piec 40 minut." {
		t.Errorf("paragraph steps = %q", got)
	}
	steps := "## Method\n\n**Step 1**\n\nMix.\n\n**Step 2**\n\nBake.\n"
	if got := strings.Join(recipeSection(steps, recipeHeadings["instructions"]), "|"); got != "Mix.|Bake." {
		t.Errorf("bold sub-headings separate steps: %q", got)
	}
	if recipeSection("no headings here", recipeHeadings["ingredients"]) != nil {
		t.Error("no section, nothing")
	}
}

func TestFillRecipeFromContent(t *testing.T) {
	g := &Generator{config: Config{RecipeHeadings: map[string][]string{"ingredients": {"What you need"}}}}
	md := "## What you need\n\n- flour\n\n## Method\n\n1. Mix.\n"
	ld := map[string]interface{}{"@type": "Recipe"}
	g.fillRecipeFromContent(ld, md)
	if len(ld["recipeIngredient"].([]interface{})) != 1 || len(ld["recipeInstructions"].([]interface{})) != 1 {
		t.Errorf("ld = %v", ld)
	}
	own := map[string]interface{}{"@type": "Recipe", "recipeIngredient": []interface{}{"butter"}}
	g.fillRecipeFromContent(own, md)
	if own["recipeIngredient"].([]interface{})[0] != "butter" || own["recipeInstructions"] == nil {
		t.Errorf("schema: wins field by field: %v", own)
	}
	article := map[string]interface{}{"@type": "BlogPosting"}
	g.fillRecipeFromContent(article, md)
	if article["recipeIngredient"] != nil {
		t.Error("only a Recipe")
	}
}

func TestFAQLD(t *testing.T) {
	g := &Generator{}
	page := models.Page{Extra: map[string]interface{}{"faq": []interface{}{
		map[string]interface{}{"question": "Can I freeze it?", "answer": "Yes, for **a month**."},
		map[string]interface{}{"question": "", "answer": "orphan"},
		"not a pair",
	}}}
	ld := g.faqLD(page)
	if ld == nil {
		t.Fatal("no FAQPage")
	}
	items := ld["mainEntity"].([]interface{})
	ans := items[0].(map[string]interface{})["acceptedAnswer"].(map[string]interface{})["text"]
	if len(items) != 1 || ans != "Yes, for a month." {
		t.Errorf("faq = %v", ld)
	}
	if g.faqLD(models.Page{}) != nil || g.faqLD(models.Page{Extra: map[string]interface{}{"faq": []interface{}{"x"}}}) != nil {
		t.Error("nothing to say, no FAQPage")
	}
}
