package models

// A content page opting into pagination over a collection it names (#267).
//
// `paginate` worked on every generated listing — the post index, category, tag,
// author, date and type archives — and on nothing a person wrote. A page that
// renders a listing of its own (an aggregated feed above the site's own posts,
// say) had no `.Pager` and no way to ask for `/blog/page/2/`: `posts_page`
// would take the URL away from the page, and a template can slice
// `.Site.Posts` but cannot write a second file. The only answer was paging in
// the browser, which is a client-side answer to a static-site question.
//
// The frontmatter form is deliberately small. A bare number is the common case:
//
//	paginate: 10
//
// and the mapping form names the collection and narrows it:
//
//	paginate:
//	  over: posts        # posts (default) | pages
//	  size: 10
//	  source: blog       # a content root, matched the way feeds: does

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// PaginateSpec is a page's request to be split over a collection.
type PaginateSpec struct {
	Over   string `yaml:"over" json:"over"`     // "posts" (default) or "pages"
	Size   int    `yaml:"size" json:"size"`     // items per page; 0 = the site's `paginate`, else 10
	Source string `yaml:"source" json:"source"` // optional content root, as in feeds: and sitemaps:
}

// UnmarshalYAML accepts both `paginate: 10` and the mapping form.
func (s *PaginateSpec) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var n int
		if err := value.Decode(&n); err != nil {
			return fmt.Errorf("paginate: want a number or a mapping, got %q", value.Value)
		}
		*s = PaginateSpec{Size: n}
		return nil
	case yaml.MappingNode:
		type plain PaginateSpec // no UnmarshalYAML, so Decode does not recurse
		var p plain
		if err := value.Decode(&p); err != nil {
			return err
		}
		*s = PaginateSpec(p)
		return nil
	}
	return fmt.Errorf("paginate: want a number or a mapping")
}

// Collection is the collection name with the default applied.
func (s PaginateSpec) Collection() string {
	if s.Over == "" {
		return "posts"
	}
	return s.Over
}
