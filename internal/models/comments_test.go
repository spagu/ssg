package models

// Reader comments carried across by a migration (#142) — the one content a
// site owner did not write and cannot re-create.

import "testing"

func comment(id, parent int, url, author string) Comment {
	return Comment{ID: id, Parent: parent, PostURL: url, Author: author, Content: "<p>hi</p>"}
}

// TestCommentsByPageThreads: comments group by the page they belong to, nest
// by parent, and stay in the order they were written so a reply never precedes
// what it answers.
func TestCommentsByPageThreads(t *testing.T) {
	file := CommentsFile{Total: 5, Comments: []Comment{
		comment(4, 2, "/blog/a/", "reply-to-2"),
		comment(1, 0, "/blog/a/", "first"),
		comment(2, 0, "/blog/a/", "second"),
		comment(5, 4, "/blog/a/", "reply-to-4"),
		comment(9, 0, "/blog/b/", "other page"),
	}}
	byPage := CommentsByPage(file)
	if len(byPage) != 2 {
		t.Fatalf("pages = %d, want 2", len(byPage))
	}

	thread := byPage["/blog/a"]
	if len(thread) != 2 || thread[0].Author != "first" || thread[1].Author != "second" {
		t.Fatalf("top level wrong: %+v", thread)
	}
	replies := thread[1].Replies
	if len(replies) != 1 || replies[0].Author != "reply-to-2" {
		t.Fatalf("reply not nested: %+v", replies)
	}
	if len(replies[0].Replies) != 1 || replies[0].Replies[0].Author != "reply-to-4" {
		t.Fatalf("nested reply lost: %+v", replies[0].Replies)
	}
}

// TestCommentsByPageKeepsOrphans: a thread exported without one of its parents
// still shows every comment it has.
func TestCommentsByPageKeepsOrphans(t *testing.T) {
	file := CommentsFile{Comments: []Comment{
		comment(3, 99, "/p/", "orphan"),
		comment(1, 0, "/p/", "root"),
		comment(7, 7, "/p/", "self-parented"),
	}}
	thread := CommentsByPage(file)["/p"]
	if len(thread) != 3 {
		t.Fatalf("no comment may vanish: %+v", thread)
	}
}

// TestCommentsByPageSkipsUnaddressable: a record with no page URL has nothing
// to attach to and must not become a phantom page.
func TestCommentsByPageSkipsUnaddressable(t *testing.T) {
	byPage := CommentsByPage(CommentsFile{Comments: []Comment{
		comment(1, 0, "", "nowhere"), comment(2, 0, "  ", "also nowhere"),
	}})
	if len(byPage) != 0 {
		t.Fatalf("unaddressable comments must be skipped: %+v", byPage)
	}
}

// TestNormalizeCommentURL: both sides of the match spell the same address
// differently — the export writes root-relative links, a page's own Link may
// carry a trailing slash — and neither spelling may cost a thread.
func TestNormalizeCommentURL(t *testing.T) {
	cases := map[string]string{
		"/blog/post/":                        "/blog/post",
		"/blog/post":                         "/blog/post",
		"blog/post/":                         "/blog/post",
		"https://example.com/blog/post/":     "/blog/post",
		"https://example.com":                "/",
		"/blog/post/?replytocom=4#comment-4": "/blog/post",
		"/blog/post/index.html":              "/blog/post/",
		"":                                   "",
		"   ":                                "",
	}
	for in, want := range cases {
		if got := NormalizeCommentURL(in); got != want {
			t.Errorf("NormalizeCommentURL(%q) = %q, want %q", in, got, want)
		}
	}
}
