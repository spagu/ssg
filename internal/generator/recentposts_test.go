// Package generator - tests for the GO-008 tmplRecentPosts bounds guard.
package generator

import (
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestTmplRecentPostsBounds verifies GO-008: the count is clamped at both ends,
// so a negative argument (e.g. {{recentPosts -1}}) cannot panic with a
// slice-bounds-out-of-range, and an oversized count is capped at the slice len.
func TestTmplRecentPostsBounds(t *testing.T) {
	g := &Generator{siteData: &models.SiteData{
		Posts: []models.Page{{Title: "a"}, {Title: "b"}, {Title: "c"}},
	}}

	tests := []struct {
		name string
		n    int
		want int
	}{
		{"negative", -1, 0},
		{"large negative", -100, 0},
		{"zero", 0, 0},
		{"within", 2, 2},
		{"exact", 3, 3},
		{"over", 100, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.tmplRecentPosts(tt.n)
			if len(got) != tt.want {
				t.Errorf("tmplRecentPosts(%d) len = %d, want %d", tt.n, len(got), tt.want)
			}
		})
	}
}
