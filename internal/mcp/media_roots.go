package mcp

// Addressing media the way the site serves it (#218).
//
// The media tools shipped knowing one root, `static/`, which is the theme's own
// and is usually empty on a migrated site. Everything wpexporter brings across
// lands under the content source's `media/` and is published at `/media/…`. So
// on exactly the sites the tools were built for — "change the picture on the
// about page" — media_list said "nothing" while the site served hundreds of
// pictures, and media_replace said "not there".
//
// The fix is not another directory in a list. It is addressing files the way
// the person asking refers to them: the owner does not know a picture is stored
// under a content source, they know it is /media/images/team.jpg. So a served
// path is the address, and which root holds it is this file's problem.

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// mediaRoots returns the roots to search, newest configuration first and
// falling back to what the tools assumed before they knew better.
func (s *Server) mediaRoots() []MediaRoot {
	if len(s.opts.MediaRoots) > 0 {
		return s.opts.MediaRoots
	}
	roots := make([]MediaRoot, 0, len(s.opts.StaticDirs))
	for _, dir := range s.opts.StaticDirs {
		roots = append(roots, MediaRoot{Dir: dir, URL: "/"})
	}
	return roots
}

// mediaBases lists the directories media may be read from or written to.
func (s *Server) mediaBases() []string {
	roots := s.mediaRoots()
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, r.Dir)
	}
	return out
}

// servedPath renders how the site addresses a stored file.
func (r MediaRoot) servedPath(rel string) string {
	stored := strings.TrimPrefix(strings.TrimPrefix(rel, r.Dir), "/")
	return path.Join("/", r.URL, stored)
}

// storedPath renders where a served path is written, or reports that this root
// does not serve it.
func (r MediaRoot) storedPath(served string) (string, bool) {
	prefix := path.Join("/", r.URL)
	if prefix != "/" {
		prefix += "/"
	}
	rest, ok := strings.CutPrefix(path.Join("/", served), prefix)
	if !ok || rest == "" {
		return "", false
	}
	return path.Join(r.Dir, rest), true
}

// rootsBySpecificity orders roots so the longest URL prefix is tried first.
//
// Every root serves under "/" in the sense that "/" is a prefix of everything,
// so a media root at /media/ would lose to the static root at / on any
// first-match search — and every upload meant for /media/ would land in the
// theme's directory instead.
func rootsBySpecificity(roots []MediaRoot) []MediaRoot {
	out := append([]MediaRoot{}, roots...)
	sort.SliceStable(out, func(a, b int) bool {
		return len(path.Join("/", out[a].URL)) > len(path.Join("/", out[b].URL))
	})
	return out
}

// resolveMedia turns whatever the caller wrote into an absolute path, the root
// that owns it, and the address the site serves it under.
//
// A served path is the documented form. A project-relative one is accepted too,
// because the tools took those when they shipped and an agent that learned them
// yesterday should not be told its own path is wrong today.
func (s *Server) resolveMedia(given string) (abs string, root MediaRoot, served string, err error) {
	given = strings.TrimSpace(given)
	if given == "" {
		return "", MediaRoot{}, "", fmt.Errorf("`path` is required")
	}
	// A ".." segment is refused rather than normalised away. path.Join would
	// clean it and resolveIn would catch whatever survived, so nothing escapes
	// either way — but a caller who writes ".." means something, and quietly
	// resolving it somewhere else leaves their model of where the file went
	// disagreeing with mine.
	for _, seg := range strings.Split(given, "/") {
		if seg == ".." {
			return "", MediaRoot{}, "", fmt.Errorf(
				"%q contains \"..\"; give the path the way the site serves it, with no relative steps", given)
		}
	}
	roots := rootsBySpecificity(s.mediaRoots())

	// Project-relative first when it plainly is one: "static/a.png" is under a
	// root directory, while "/static/a.png" is a served path that probably is
	// not served at all. Checked before the served form so a directory named
	// like a URL prefix cannot make the two ambiguous.
	if !strings.HasPrefix(given, "/") {
		for _, r := range roots {
			if given == r.Dir || strings.HasPrefix(given, r.Dir+"/") {
				abs, _, resolveErr := resolveIn(s.opts.Root, []string{r.Dir}, given)
				if resolveErr != nil {
					return "", MediaRoot{}, "", resolveErr
				}
				return abs, r, r.servedPath(given), nil
			}
		}
	}

	// Only something written as a URL is read as one. Without this, a path that
	// belongs to no root — "content/posts/x.png", or "../escape.png" — was
	// quietly reinterpreted as a served path and written into the first root
	// that could hold it. Confined, but not what anyone asked for, and a tool
	// that silently relocates a file is worse than one that refuses.
	if !strings.HasPrefix(given, "/") {
		return "", MediaRoot{}, "", unknownMediaPath(given, s.mediaRoots())
	}
	for _, r := range roots {
		stored, ok := r.storedPath(given)
		if !ok {
			continue
		}
		abs, _, resolveErr := resolveIn(s.opts.Root, []string{r.Dir}, stored)
		if resolveErr != nil {
			return "", MediaRoot{}, "", resolveErr
		}
		return abs, r, r.servedPath(stored), nil
	}

	return "", MediaRoot{}, "", unknownMediaPath(given, s.mediaRoots())
}

// unknownMediaPath explains where media do live, so the next attempt is an
// informed one rather than another guess.
func unknownMediaPath(given string, roots []MediaRoot) error {
	return fmt.Errorf(
		"%q is not somewhere this site publishes media. They are served under: %s — "+
			"call media_list to see what is there, and address a file the way the site does",
		given, strings.Join(servedPrefixes(roots), ", "))
}

// servedPrefixes names the URL prefixes media are served under, for a message
// that tells the reader where to look instead.
func servedPrefixes(roots []MediaRoot) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		prefix := path.Join("/", r.URL)
		if prefix != "/" {
			prefix += "/"
		}
		if !seen[prefix] {
			seen[prefix] = true
			out = append(out, prefix)
		}
	}
	sort.Strings(out)
	return out
}
