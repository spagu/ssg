package generator

// Archives for a custom post type (#165).
//
// A migration brings a WordPress custom post type across as a folder of
// documents, each at the address the source served. What it cannot bring across
// is the type's archive: `/realizacje/` is not a document anywhere, it is a
// listing WordPress renders from `has_archive`. So the entries build, the site's
// own menu links to the section, and the section is a 404.
//
// The generator already makes exactly this shape for categories, tags and dates.
// The only thing missing was being told which types deserve one — and that
// cannot be guessed: on the site that reported it, a second custom type
// (`reviews`) has no archive and the SOURCE 404s there too. Inventing one would
// publish a page the original never had.
//
// So it is declared, and nothing happens without a declaration:
//
//	type_archives:
//	  realizacje: true
//	  reviews: false
//
// An export answers for itself: metadata.json's custom_types[] carries
// has_archive and archive_link since wpexporter 1.8.15, so a migrated project
// needs no configuration at all. The map stays for older exports, and for
// overruling one.

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// typeArchive is one custom post type's listing: the address it takes, the name
// a template titles it with, and the documents it lists.
type typeArchive struct {
	// Type is the content type's slug, as written in each document's `type:`.
	Type string
	// Path is where the listing is published. Usually the type's own slug, but
	// WordPress lets `has_archive` BE a slug — a type called `realizacje` can
	// serve its archive at /nasze-prace/ — and building it anywhere else would
	// publish a section nothing links to.
	Path  string
	Name  string
	Pages []models.Page
}

// generateTypeArchives writes one listing per declared content type. A no-op for
// a site that declares none, which is every site that has not asked.
func (g *Generator) generateTypeArchives() error {
	archives := g.collectTypeArchives()
	written := 0
	for _, a := range archives {
		ok, err := g.writeTypeArchive(a)
		if err != nil {
			return err
		}
		if ok {
			written++
		}
	}
	if written > 0 && !g.config.Quiet {
		fmt.Printf("   🗂️  Generated %d content-type archive(s)\n", written)
	}
	return nil
}

// collectTypeArchives groups content by type, keeping only the types that were
// declared to have an archive and that actually have entries. An archive of
// nothing is a page nobody linked.
func (g *Generator) collectTypeArchives() []typeArchive {
	wanted := g.archivedTypes()
	if len(wanted) == 0 || g.siteData == nil {
		return nil
	}

	byType := map[string][]models.Page{}
	collect := func(pages []models.Page) {
		for _, p := range pages {
			key := strings.ToLower(strings.TrimSpace(p.Type))
			if _, ok := wanted[key]; ok {
				byType[key] = append(byType[key], p)
			}
		}
	}
	collect(g.siteData.Pages)
	collect(g.siteData.Posts)

	out := make([]typeArchive, 0, len(byType))
	for slug, pages := range byType {
		out = append(out, typeArchive{
			Type:  slug,
			Path:  g.typeArchivePath(slug),
			Name:  wanted[slug],
			Pages: sortPostsByDate(pages),
		})
	}
	// Deterministic order so a build is reproducible and the log reads the same
	// way twice.
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// typeArchivePath returns where a type's listing is published: the address the
// export recorded for it, or the type's own slug when it recorded none.
func (g *Generator) typeArchivePath(typeSlug string) string {
	for _, t := range g.siteData.CustomTypes {
		if !strings.EqualFold(strings.TrimSpace(t.Slug), typeSlug) {
			continue
		}
		if p := archiveLinkPath(t.ArchiveLink); p != "" {
			return p
		}
		break
	}
	return typeSlug
}

// archiveLinkPath reduces a recorded archive address to the output path it
// names. wpexporter writes it root-relative ("/nasze-prace/"), but a hand-edited
// metadata.json may hold a full URL, and neither should end up as a directory
// called "https:".
func archiveLinkPath(link string) string {
	s := strings.TrimSpace(link)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = u.Path
	}
	return strings.Trim(s, "/")
}

// archivedTypes returns the types that should get an archive, mapped to the name
// a template titles them with.
//
// A type is included when the config says true, or when the loaded metadata says
// the source served an archive for it. An explicit `false` wins over the
// metadata: the operator has looked, and the reported site is exactly the case
// where one custom type has an archive and another does not.
func (g *Generator) archivedTypes() map[string]string {
	out := map[string]string{}
	for _, t := range g.siteData.CustomTypes {
		slug := strings.ToLower(strings.TrimSpace(t.Slug))
		if slug == "" || !t.HasArchive {
			continue
		}
		out[slug] = typeArchiveName(t.Name, slug)
	}
	for slug, want := range g.config.TypeArchives {
		key := strings.ToLower(strings.TrimSpace(slug))
		if key == "" {
			continue
		}
		if !want {
			delete(out, key)
			continue
		}
		if _, ok := out[key]; !ok {
			out[key] = typeArchiveName("", key)
		}
	}
	return out
}

// typeArchiveName prefers the name the source CMS used, falling back to the slug
// with its separators turned into spaces — a readable title either way.
func typeArchiveName(name, slug string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(slug, "-", " "), "_", " "))
}

// writeTypeArchive renders one listing at /<type>/, unless real content already
// owns that URL — a hand-written section page outranks a generated listing, the
// same rule every other archive follows.
func (g *Generator) writeTypeArchive(a typeArchive) (bool, error) {
	slug := models.SanitizeRelPath(a.Path)
	if slug == "" {
		return false, nil
	}
	if owner, taken := g.archivePathOwner(slug); taken {
		if !g.config.Quiet {
			fmt.Printf("   ⚠️  Skipping %s archive /%s/: %s already owns that URL\n", a.Type, slug, owner)
		}
		return false, nil
	}

	root := filepath.Join(g.config.OutputDir, filepath.FromSlash(slug))
	term := models.Category{Name: a.Name, Slug: slug}
	for _, chunk := range paginateTerm(a.Pages, g.archivePerPage(), "/"+slug+"/") {
		outputPath := filepath.Join(root, indexHTMLName)
		if chunk.Pager.Current > 1 {
			outputPath = filepath.Join(root, "page", fmt.Sprintf("%d", chunk.Pager.Current), indexHTMLName)
		}
		if err := g.ensureWithinOutput(outputPath); err != nil {
			fmt.Printf("   ⚠️  Skipping %s archive with unsafe slug: %v\n", a.Type, err)
			return false, nil
		}
		if err := g.ensureParent(outputPath); err != nil {
			return false, err
		}
		data := g.archiveData("type", a.Name, term, chunk.Posts, chunk.Pager, g.currentLang)
		// The type slug lets a theme tell one content-type archive from another
		// without parsing the URL.
		data["ContentType"] = a.Type
		if err := g.renderTemplate(categoryHTMLName, outputPath, data); err != nil {
			fmt.Printf("   ⚠️  Warning: failed to generate %s archive: %v\n", a.Type, err)
			return false, nil
		}
	}
	return true, nil
}
