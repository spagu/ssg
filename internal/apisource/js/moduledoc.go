package js

import "github.com/spagu/ssg/internal/apimodel"

// moduleTags mark a doc comment as the description of its whole file.
var moduleTags = map[string]bool{
	"module": true, "file": true, "fileoverview": true, "overview": true, "packageDocumentation": true,
}

// moduleDoc returns the description of a file: its first doc comment that
// carries @module, @file, @fileoverview, @overview or
// @packageDocumentation, without that tag. The tag's text serves as the
// summary when the comment has no description. Nil when there is none.
func moduleDoc(f *file) *apimodel.Doc {
	for i := range f.scan.docs {
		c := f.docComment(i)
		doc := *c.Doc
		doc.Tags = nil
		found := false
		for _, t := range c.Doc.Tags {
			switch {
			case !moduleTags[t.Name]:
				doc.Tags = append(doc.Tags, t)
			case doc.Summary == "":
				doc.Summary, found = t.Text, true
			default:
				found = true
			}
		}
		if found {
			return &doc
		}
	}
	return nil
}
