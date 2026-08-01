package generator

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/notify"
)

// sendNotifications announces new or changed posts to the configured webhook
// destinations once a build has succeeded. A no-op unless --notify is set with
// destinations; the notifier's committed state dedupes, so a post is announced
// once — again only when its content changes (#1.8.16).
func (g *Generator) sendNotifications() error {
	if g.config.Notify == nil || !g.config.Notify.Enabled() {
		return nil
	}
	posts := make([]notify.Post, 0, len(g.siteData.Posts))
	for _, p := range g.siteData.Posts {
		excerpt := p.Excerpt
		if excerpt == "" {
			excerpt = p.Description
		}
		posts = append(posts, notify.Post{
			Slug:    p.Slug,
			Title:   p.Title,
			URL:     p.GetCanonical(g.config.Domain),
			Excerpt: excerpt,
			Date:    p.Date.Format("2006-01-02"),
			Tags:    p.Tags,
			Hash:    postHash(p),
		})
	}
	_, err := g.config.Notify.Send(posts, g.config.Quiet)
	return err
}

// postHash is the dedup key: a content change (title, body or date) re-announces
// the post; an unchanged post is skipped.
func postHash(p models.Page) string {
	h := sha256.Sum256([]byte(p.Title + "\x00" + p.Content + "\x00" + p.Date.String()))
	return hex.EncodeToString(h[:])
}
