package ai

import "testing"

// TestEvalIfs covers the ifs guard: empty is always true, AND/OR precedence,
// comparison operators (string + numeric), contains, and a parse error.
func TestEvalIfs(t *testing.T) {
	vars := map[string]string{"lang": "en", "status": "publish", "category": "news", "weight": "5", "title": "Hello World"}
	cases := []struct {
		ifs  string
		want bool
	}{
		{"", true},
		{"lang == en", true},
		{"lang == pl", false},
		{"lang != pl", true},
		{"lang == en AND status == publish", true},
		{"lang == en AND status == draft", false},
		{"lang == pl OR category == news", true},
		{"lang == pl OR category == blog", false},
		{"weight > 3", true},
		{"weight < 3", false},
		{"weight >= 5", true},
		{`title contains World`, true},
		{`title contains Nope`, false},
		{`lang == "en"`, true}, // quoted value
		{"title > A", true},    // lexicographic
		{"lang < aa", false},   // lexicographic
		{"lang >= en", true}}
	for _, c := range cases {
		got, err := Eval(c.ifs, vars)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", c.ifs, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.ifs, got, c.want)
		}
	}
	if _, err := Eval("garbage-no-operator", vars); err == nil {
		t.Error("unparsable condition must error")
	}
}
