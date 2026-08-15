package generator

// Rendering the comments a migration carried across (#142). The file reaches
// the project addressed by page URL; the generator loads it once and hands
// each page its own thread as .Comments.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spagu/ssg/internal/models"
)

// loadComments reads content/<source>/comments.json when it exists. A missing
// file is the normal shape of a site that never had comments; an unreadable
// one is reported and skipped, because a malformed archive of old comments is
// not worth failing a build over.
func (g *Generator) loadComments(path string) {
	raw, err := os.ReadFile(path) // #nosec G304 -- the project's own content directory
	if err != nil {
		return
	}
	var file models.CommentsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		g.log("   ⚠️  comments.json could not be read: " + err.Error())
		return
	}
	if len(file.Comments) == 0 {
		return
	}
	g.siteData.Comments = models.CommentsByPage(file)
	g.log(fmt.Sprintf("   💬 Loaded %d reader comment(s) for %d page(s)",
		len(file.Comments), len(g.siteData.Comments)))
}

// commentsFor returns a page's own thread, matched on the URL the export
// recorded. Nil when the page has none, so `{{with .Comments}}` renders
// nothing rather than an empty section.
func (g *Generator) commentsFor(page models.Page) []models.Comment {
	if len(g.siteData.Comments) == 0 {
		return nil
	}
	return g.siteData.Comments[models.NormalizeCommentURL(page.GetURL())]
}
