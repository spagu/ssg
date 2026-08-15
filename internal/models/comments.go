package models

// Reader comments carried across by a migration (#142).
//
// Comments are the one part of a site its owner did not write and cannot
// re-create. wpexporter brings them into content/<source>/comments.json,
// addressed by page URL rather than by WordPress's post id — which means
// nothing after a migration — and until now the file reached the project and
// stopped there: counted in the report, then ignored by the generator.
//
// They are rendered statically because that is what they are: a historical,
// closed thread. Accepting NEW comments stays the job of the D1 comments
// worker.

import (
	"sort"
	"strings"
)

// CommentsFile is the shape wpexporter writes.
type CommentsFile struct {
	Total    int       `json:"total"`
	Pages    int       `json:"pages"`
	Comments []Comment `json:"comments"`
}

// Comment is one reader's comment. PostURL addresses the page it belongs to in
// the same form the page's own Link takes, so matching them is a lookup rather
// than a heuristic.
type Comment struct {
	ID      int    `json:"id"`
	Post    int    `json:"post"`
	Parent  int    `json:"parent"`
	PostURL string `json:"post_url"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Content string `json:"content"`
	Status  string `json:"status"`

	// Replies is the resolved thread, filled by CommentsByPage.
	Replies []Comment `json:"-"`
}

// CommentsByPage groups comments by the page URL they belong to, with each
// page's thread nested by Parent and ordered by id — the order they were
// written, so a reply never precedes what it answers.
//
// A comment whose parent is missing from the same page is kept at the top
// level rather than dropped: a partially exported thread still shows every
// comment it has.
func CommentsByPage(file CommentsFile) map[string][]Comment {
	byPage := map[string][]Comment{}
	for _, c := range file.Comments {
		key := NormalizeCommentURL(c.PostURL)
		if key == "" {
			continue // nothing to attach it to
		}
		byPage[key] = append(byPage[key], c)
	}
	for key, comments := range byPage {
		byPage[key] = threadComments(comments)
	}
	return byPage
}

// threadComments nests one page's comments by Parent, id-ordered at every level.
func threadComments(comments []Comment) []Comment {
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].ID < comments[j].ID })

	type node struct {
		comment  Comment
		children []*node
	}
	nodes := make(map[int]*node, len(comments))
	flat := make([]*node, 0, len(comments))
	for _, c := range comments {
		c.Replies = nil
		n := &node{comment: c}
		nodes[c.ID] = n
		flat = append(flat, n)
	}

	var roots []*node
	for _, n := range flat {
		parent, ok := nodes[n.comment.Parent]
		if n.comment.Parent == 0 || !ok || parent == n {
			roots = append(roots, n)
			continue
		}
		parent.children = append(parent.children, n)
	}

	var build func([]*node) []Comment
	build = func(ns []*node) []Comment {
		if len(ns) == 0 {
			return nil
		}
		out := make([]Comment, 0, len(ns))
		for _, n := range ns {
			c := n.comment
			c.Replies = build(n.children)
			out = append(out, c)
		}
		return out
	}
	return build(roots)
}

// NormalizeCommentURL reduces a page address to the form both sides can be
// compared in: a leading slash, no trailing slash, no query or fragment. The
// export writes root-relative links; a page's own Link may or may not carry
// the trailing slash, and neither spelling should cost a thread.
func NormalizeCommentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	// An absolute URL keeps only its path: the host is the site being migrated
	// off, which is not where these pages live now.
	if i := strings.Index(raw, "://"); i >= 0 {
		if slash := strings.Index(raw[i+3:], "/"); slash >= 0 {
			raw = raw[i+3+slash:]
		} else {
			raw = "/"
		}
	}
	raw = "/" + strings.Trim(raw, "/")
	return strings.TrimSuffix(raw, "index.html")
}
