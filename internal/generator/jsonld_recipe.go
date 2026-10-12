package generator

// A recipe's category from the post's own categories (1.8.72). A food site
// files a cheesecake under "Cakes" already; repeating it as recipeCategory in
// every post's schema: is the duplication schema_defaults exists to remove.
// The categories fill recipeCategory only when nothing above them set it, so
// a section default or the page's own schema still wins.

import "github.com/spagu/ssg/internal/models"

// fillRecipeCategory sets recipeCategory on a Recipe from the page's
// categories: one name as a string, several as a list.
func (g *Generator) fillRecipeCategory(ld map[string]interface{}, page models.Page) {
	if !hasLDType(ld, "Recipe") || hasLDProp(ld, "recipeCategory") {
		return
	}
	names := uniqueStrings(g.categoryNames(&page))
	switch len(names) {
	case 0:
	case 1:
		ld["recipeCategory"] = names[0]
	default:
		list := make([]interface{}, len(names))
		for i, n := range names {
			list[i] = n
		}
		ld["recipeCategory"] = list
	}
}

// hasLDType reports whether a node's @type is want, as a string or as one
// member of a list (["Recipe", "HowTo"]).
func hasLDType(ld map[string]interface{}, want string) bool {
	switch t := ld["@type"].(type) {
	case string:
		return t == want
	case []interface{}:
		for _, v := range t {
			if s, ok := v.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// uniqueStrings drops repeats and blanks, keeping the first order.
func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
