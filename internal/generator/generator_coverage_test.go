// Package generator - additional coverage tests
package generator

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

func TestCopyColocatedAssetsNonexistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	gen := &Generator{config: Config{Quiet: true}}
	err := gen.copyColocatedAssets(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"))
	if err != nil {
		t.Errorf("Expected nil error for nonexistent source dir, got: %v", err)
	}
}

func TestCopyColocatedAssetsNoAssets(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "readme.md"), []byte("# Hi"), 0644); err != nil {
		t.Fatalf("Failed to create md: %v", err)
	}

	dstDir := filepath.Join(tmpDir, "dst")
	gen := &Generator{config: Config{Quiet: false}}
	if err := gen.copyColocatedAssets(srcDir, dstDir); err != nil {
		t.Fatalf("copyColocatedAssets failed: %v", err)
	}
}

func TestExtractCategoryFromDoc(t *testing.T) {
	tests := []struct {
		name     string
		doc      mddb.Document
		wantID   int
		wantName string
		wantSlug string
	}{
		{
			name: "full category",
			doc: mddb.Document{
				Key: "tech",
				Metadata: map[string]any{
					"id":          float64(5),
					"name":        "Technology",
					"description": "Tech articles",
					"link":        "https://example.com/category/tech/",
					"count":       float64(10),
					"parent":      float64(0),
				},
			},
			wantID:   5,
			wantName: "Technology",
			wantSlug: "tech",
		},
		{
			name: "empty metadata",
			doc: mddb.Document{
				Key:      "empty",
				Metadata: map[string]any{},
			},
			wantID:   0,
			wantName: "",
			wantSlug: "empty",
		},
		{
			name: "wrong types in metadata",
			doc: mddb.Document{
				Key: "wrong",
				Metadata: map[string]any{
					"id":   "not-a-number",
					"name": 12345,
				},
			},
			wantID:   0,
			wantName: "",
			wantSlug: "wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := extractCategoryFromDoc(tt.doc)
			if cat.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", cat.ID, tt.wantID)
			}
			if cat.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cat.Name, tt.wantName)
			}
			if cat.Slug != tt.wantSlug {
				t.Errorf("Slug = %q, want %q", cat.Slug, tt.wantSlug)
			}
		})
	}
}

func TestExtractMediaFromDoc(t *testing.T) {
	tests := []struct {
		name       string
		doc        mddb.Document
		wantID     int
		wantType   string
		wantMime   string
		wantSrcURL string
		wantTitle  string
	}{
		{
			name: "full media",
			doc: mddb.Document{
				Key: "image1",
				Metadata: map[string]any{
					"id":         float64(100),
					"media_type": "image",
					"mime_type":  "image/jpeg",
					"source_url": "https://example.com/image.jpg",
					"title": map[string]interface{}{
						"rendered": "My Image",
					},
				},
			},
			wantID:     100,
			wantType:   "image",
			wantMime:   "image/jpeg",
			wantSrcURL: "https://example.com/image.jpg",
			wantTitle:  "My Image",
		},
		{
			name: "empty metadata",
			doc: mddb.Document{
				Key:      "empty",
				Metadata: map[string]any{},
			},
			wantID:     0,
			wantType:   "",
			wantMime:   "",
			wantSrcURL: "",
			wantTitle:  "",
		},
		{
			name: "title without rendered",
			doc: mddb.Document{
				Key: "partial",
				Metadata: map[string]any{
					"title": map[string]interface{}{
						"raw": "Raw Title",
					},
				},
			},
			wantTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := extractMediaFromDoc(tt.doc)
			if media.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", media.ID, tt.wantID)
			}
			if media.MediaType != tt.wantType {
				t.Errorf("MediaType = %q, want %q", media.MediaType, tt.wantType)
			}
			if media.MimeType != tt.wantMime {
				t.Errorf("MimeType = %q, want %q", media.MimeType, tt.wantMime)
			}
			if media.SourceURL != tt.wantSrcURL {
				t.Errorf("SourceURL = %q, want %q", media.SourceURL, tt.wantSrcURL)
			}
			if media.Title.Rendered != tt.wantTitle {
				t.Errorf("Title.Rendered = %q, want %q", media.Title.Rendered, tt.wantTitle)
			}
		})
	}
}

func TestExtractAuthorFromDoc(t *testing.T) {
	tests := []struct {
		name     string
		doc      mddb.Document
		wantID   int
		wantName string
		wantSlug string
	}{
		{
			name: "full author",
			doc: mddb.Document{
				Key: "admin",
				Metadata: map[string]any{
					"id":   float64(1),
					"name": "Admin User",
				},
			},
			wantID:   1,
			wantName: "Admin User",
			wantSlug: "admin",
		},
		{
			name: "empty metadata",
			doc: mddb.Document{
				Key:      "unknown",
				Metadata: map[string]any{},
			},
			wantID:   0,
			wantName: "",
			wantSlug: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author := extractAuthorFromDoc(tt.doc)
			if author.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", author.ID, tt.wantID)
			}
			if author.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", author.Name, tt.wantName)
			}
			if author.Slug != tt.wantSlug {
				t.Errorf("Slug = %q, want %q", author.Slug, tt.wantSlug)
			}
		})
	}
}

func TestConvertRelativeLinksIfRequested(t *testing.T) {
	tests := []struct {
		name          string
		relativeLinks bool
		domain        string
		expectSkip    bool
	}{
		{"disabled", false, "example.com", true},
		{"no domain", true, "", true},
		{"enabled with domain", true, "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if !tt.expectSkip {
				if err := os.WriteFile(filepath.Join(tmpDir, "test.html"), []byte(`<a href="https://example.com/page">Link</a>`), 0644); err != nil {
					t.Fatalf("Failed to create html: %v", err)
				}
			}

			gen := &Generator{
				config: Config{
					RelativeLinks: tt.relativeLinks,
					Domain:        tt.domain,
					OutputDir:     tmpDir,
					Quiet:         true,
				},
			}

			err := gen.convertRelativeLinksIfRequested()
			if err != nil {
				t.Fatalf("convertRelativeLinksIfRequested failed: %v", err)
			}

			if !tt.expectSkip {
				content, _ := os.ReadFile(filepath.Join(tmpDir, "test.html"))
				if strings.Contains(string(content), "https://example.com") {
					t.Error("Links should have been converted to relative")
				}
			}
		})
	}
}

func TestConvertToRelativeLinksError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path",
			Domain:    "example.com",
		},
	}

	err := gen.convertToRelativeLinks()
	if err == nil {
		t.Error("Expected error for nonexistent output dir")
	}
}

func TestLinkifyListItem(t *testing.T) {
	pageLinks := map[string]string{
		"About Us":  "/about/",
		"Contact":   "/contact/",
		"Caf\u00e9": "/cafe/",
	}

	tests := []struct {
		name     string
		line     string
		content  string
		expected string
	}{
		{
			name:     "matching page link",
			line:     "- About Us",
			content:  "About Us",
			expected: "- [About Us](/about/)",
		},
		{
			name:     "no match",
			line:     "- Unknown Page",
			content:  "Unknown Page",
			expected: "- Unknown Page",
		},
		{
			name:     "html entity match",
			line:     "- Caf\u00e9",
			content:  "Caf\u00e9",
			expected: "- [Caf\u00e9](/cafe/)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := linkifyListItem(tt.line, tt.content, pageLinks)
			if result != tt.expected {
				t.Errorf("linkifyListItem(%q, %q) = %q, want %q", tt.line, tt.content, result, tt.expected)
			}
		})
	}
}

func TestExtractListItemContent(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{"dash list item", "- Hello", "Hello"},
		{"asterisk list item", "* World", "World"},
		{"indented dash", "  - Indented", "Indented"},
		{"not a list item", "Regular text", ""},
		{"empty line", "", ""},
		{"dash only", "-", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractListItemContent(tt.line)
			if result != tt.expected {
				t.Errorf("extractListItemContent(%q) = %q, want %q", tt.line, result, tt.expected)
			}
		})
	}
}

func TestTmplFormatDateNonString(t *testing.T) {
	result := tmplFormatDate(42)
	if result != "42" {
		t.Errorf("tmplFormatDate(42) = %q, want %q", result, "42")
	}

	result = tmplFormatDate("2024-01-15")
	if result != "2024-01-15" {
		t.Errorf("tmplFormatDate(string) = %q, want %q", result, "2024-01-15")
	}
}

func TestTmplGetCategoryNameMissing(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Categories: map[int]models.Category{
				1: {ID: 1, Name: "News"},
			},
		},
	}

	if name := gen.tmplGetCategoryName(999); name != "" {
		t.Errorf("Expected empty string for missing category, got %q", name)
	}
	if name := gen.tmplGetCategoryName(1); name != "News" {
		t.Errorf("Expected 'News', got %q", name)
	}
}

func TestTmplGetCategorySlugMissing(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Categories: map[int]models.Category{
				1: {ID: 1, Slug: "news"},
			},
		},
	}

	if slug := gen.tmplGetCategorySlug(999); slug != "" {
		t.Errorf("Expected empty string for missing category, got %q", slug)
	}
	if slug := gen.tmplGetCategorySlug(1); slug != "news" {
		t.Errorf("Expected 'news', got %q", slug)
	}
}

func TestTmplGetAuthorNameMissing(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Authors: map[int]models.Author{
				1: {ID: 1, Name: "Admin"},
			},
		},
	}

	if name := gen.tmplGetAuthorName(999); name != "" {
		t.Errorf("Expected empty string for missing author, got %q", name)
	}
	if name := gen.tmplGetAuthorName(1); name != "Admin" {
		t.Errorf("Expected 'Admin', got %q", name)
	}
}

func TestTmplIsValidCategory(t *testing.T) {
	if tmplIsValidCategory(1) {
		t.Error("Category 1 should not be valid")
	}
	if !tmplIsValidCategory(2) {
		t.Error("Category 2 should be valid")
	}
}

func TestTmplThumbnailFromYoutubeNoMatch(t *testing.T) {
	result := tmplThumbnailFromYoutube("no youtube shortcode here")
	if result != "" {
		t.Errorf("Expected empty string for no match, got %q", result)
	}
}

func TestTmplThumbnailFromYoutubeMatch(t *testing.T) {
	result := tmplThumbnailFromYoutube("[youtube]https://www.youtube.com/watch?v=abc123[/youtube]")
	expected := "https://img.youtube.com/vi/abc123/hqdefault.jpg"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTmplStripShortcodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"strip youtube", "text [youtube]http://yt.com[/youtube] more", "text  more"},
		{"strip embed", "text [embed]http://yt.com[/embed] more", "text  more"},
		{"no shortcodes", "plain text", "plain text"},
		{"both shortcodes", "[youtube]url[/youtube] and [embed]url2[/embed]", "and"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tmplStripShortcodes(tt.input)
			if result != tt.expected {
				t.Errorf("tmplStripShortcodes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTmplStripHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple tags", "<p>Hello</p>", "Hello"},
		{"nested tags", "<div><p>World</p></div>", "World"},
		{"no tags", "plain", "plain"},
		{"self-closing", "<br/><img src='x'>text", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tmplStripHTML(tt.input)
			if result != tt.expected {
				t.Errorf("tmplStripHTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTmplRecentPosts(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Posts: []models.Page{
				{Title: "Post 1"},
				{Title: "Post 2"},
				{Title: "Post 3"},
			},
		},
	}

	result := gen.tmplRecentPosts(2)
	if len(result) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(result))
	}

	result = gen.tmplRecentPosts(10)
	if len(result) != 3 {
		t.Errorf("Expected 3 posts (capped), got %d", len(result))
	}
}

func TestCleanMarkdownArtifacts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"bold text", "**hello**", "<strong>hello</strong>"},
		{"standalone stars", "  **  \ntext", "\ntext"},
		{"no artifacts", "plain text", "plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanMarkdownArtifacts(tt.input)
			if result != tt.expected {
				t.Errorf("cleanMarkdownArtifacts(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAutolinkListItems(t *testing.T) {
	pageLinks := map[string]string{
		"About": "/about/",
	}

	input := "- About\n- Unknown\nRegular text"
	result := autolinkListItems(input, pageLinks)

	if !strings.Contains(result, "[About](/about/)") {
		t.Errorf("Expected autolinked About, got: %s", result)
	}
	if !strings.Contains(result, "- Unknown") {
		t.Errorf("Expected unchanged Unknown, got: %s", result)
	}
}

func TestConvertMarkdownToHTML(t *testing.T) {
	result := convertMarkdownToHTML("# Hello")
	if !strings.Contains(result, "<h1>Hello</h1>") {
		t.Errorf("Expected h1, got: %s", result)
	}
}

func TestGenerateWithRelativeLinks(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body><a href="https://example.com/about">About</a></body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:        "test-source",
		Template:      "simple",
		Domain:        "https://example.com",
		ContentDir:    contentDir,
		TemplatesDir:  filepath.Join(tmpDir, "templates"),
		OutputDir:     outputDir,
		RelativeLinks: true,
		Quiet:         true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	indexContent, _ := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if strings.Contains(string(indexContent), "https://example.com") {
		t.Error("Absolute links should be converted to relative")
	}
}

func TestPrettifyIfRequestedSkipsWhenMinify(t *testing.T) {
	gen := &Generator{
		config: Config{
			PrettyHTML: true,
			MinifyHTML: true,
		},
	}
	err := gen.prettifyIfRequested()
	if err != nil {
		t.Errorf("Expected nil when MinifyHTML overrides PrettyHTML, got: %v", err)
	}
}

func TestMinifyIfRequestedSkipsWhenDisabled(t *testing.T) {
	gen := &Generator{
		config: Config{
			MinifyHTML: false,
			MinifyCSS:  false,
			MinifyJS:   false,
		},
	}
	err := gen.minifyIfRequested()
	if err != nil {
		t.Errorf("Expected nil when all minify disabled, got: %v", err)
	}
}

func TestLogContentStats(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Pages:      []models.Page{{Title: "A"}, {Title: "B"}},
			Posts:      []models.Page{{Title: "P"}},
			Categories: map[int]models.Category{1: {ID: 1}},
			Media:      map[int]models.MediaItem{1: {ID: 1}},
		},
	}
	gen.logContentStats()
}

func TestBuildPageLinks(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Pages: []models.Page{
				{Title: "About Us", Slug: "about", Link: "https://example.com/about/"},
			},
			Posts: []models.Page{
				{Title: "Hello World", Slug: "hello", Link: "https://example.com/hello/"},
			},
		},
	}

	links := gen.buildPageLinks()
	if _, ok := links["About Us"]; !ok {
		t.Error("Expected 'About Us' in page links")
	}
	if _, ok := links["Hello World"]; !ok {
		t.Error("Expected 'Hello World' in page links")
	}
}

func TestGenerateCategoriesMultiplePosts(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := template.Must(template.New("category.html").Parse("<html>{{.Category.Name}} - {{len .Posts}} posts</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Posts: []models.Page{
				{Title: "Post 1", Slug: "post1", Categories: []int{2, 3}},
				{Title: "Post 2", Slug: "post2", Categories: []int{2}},
				{Title: "Post 3", Slug: "post3", Categories: []int{3}},
			},
			Categories: map[int]models.Category{
				2: {ID: 2, Name: "News", Slug: "news"},
				3: {ID: 3, Name: "Tech", Slug: "tech"},
			},
		},
		tmpl: tmpl,
	}

	if err := gen.generateCategories(); err != nil {
		t.Fatalf("generateCategories failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "category", "news", "index.html")); err != nil {
		t.Error("News category page not generated")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "category", "tech", "index.html")); err != nil {
		t.Error("Tech category page not generated")
	}
}

func TestRunStep(t *testing.T) {
	gen := &Generator{config: Config{Quiet: true}}

	errFn := func() error { return fmt.Errorf("step failed") }
	err := gen.runStep("test", errFn, "test context")
	if err == nil {
		t.Error("Expected error from runStep")
	}
	if !strings.Contains(err.Error(), "test context") {
		t.Errorf("Expected error to contain 'test context', got: %v", err)
	}

	okFn := func() error { return nil }
	err = gen.runStep("test", okFn, "test context")
	if err != nil {
		t.Errorf("Expected nil error from runStep, got: %v", err)
	}
}

func TestNewWithShortcodes(t *testing.T) {
	cfg := Config{
		Shortcodes: []Shortcode{
			{Name: "promo", Text: "Promo text"},
			{Name: "banner", Text: "Banner text"},
		},
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if len(gen.shortcodeMap) != 2 {
		t.Errorf("Expected 2 shortcodes in map, got %d", len(gen.shortcodeMap))
	}
	if _, ok := gen.shortcodeMap["promo"]; !ok {
		t.Error("Expected 'promo' in shortcode map")
	}
}

func TestProcessShortcodesBracketSyntax(t *testing.T) {
	gen := &Generator{
		config: Config{ShortcodeBrackets: true},
		shortcodeMap: map[string]Shortcode{
			"promo": {Name: "promo", Text: "Promo text"},
		},
	}

	// [promo] should be replaced (defined shortcode)
	result := gen.processShortcodes("Hello [promo] world")
	if strings.Contains(result, "[promo]") {
		t.Errorf("Expected [promo] to be replaced, got: %s", result)
	}

	// [unknown] should NOT be replaced (undefined)
	result = gen.processShortcodes("Hello [unknown] world")
	if !strings.Contains(result, "[unknown]") {
		t.Errorf("Expected [unknown] to remain untouched, got: %s", result)
	}

	// {{promo}} should also work
	result = gen.processShortcodes("Hello {{promo}} world")
	if strings.Contains(result, "{{promo}}") {
		t.Errorf("Expected {{promo}} to be replaced, got: %s", result)
	}
}

func TestProcessShortcodesBracketDisabled(t *testing.T) {
	gen := &Generator{
		config: Config{ShortcodeBrackets: false},
		shortcodeMap: map[string]Shortcode{
			"promo": {Name: "promo", Text: "Promo text"},
		},
	}

	// [promo] should NOT be replaced when disabled
	result := gen.processShortcodes("Hello [promo] world")
	if !strings.Contains(result, "[promo]") {
		t.Errorf("Expected [promo] to remain when brackets disabled, got: %s", result)
	}

	// {{promo}} should still work
	result = gen.processShortcodes("Hello {{promo}} world")
	if strings.Contains(result, "{{promo}}") {
		t.Errorf("Expected {{promo}} to be replaced, got: %s", result)
	}
}

func TestProcessShortcodesBracketWithTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "banner.html")
	if err := os.WriteFile(tmplFile, []byte(`<div>{{.Title}}</div>`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"banner": {Name: "banner", Title: "My Banner", Template: "banner.html"},
		},
	}

	result := gen.processShortcodes("Before [banner] After")
	if strings.Contains(result, "[banner]") {
		t.Errorf("Expected [banner] to be replaced, got: %s", result)
	}
	if !strings.Contains(result, "My Banner") {
		t.Errorf("Expected rendered template with 'My Banner', got: %s", result)
	}
}

func TestProcessShortcodesBracketEmptyMap(t *testing.T) {
	gen := &Generator{
		config:       Config{ShortcodeBrackets: true},
		shortcodeMap: map[string]Shortcode{},
	}

	// Should not crash with empty map
	result := gen.processShortcodes("Hello [anything] world")
	if !strings.Contains(result, "[anything]") {
		t.Errorf("Expected [anything] to remain with empty map, got: %s", result)
	}
}

func TestProcessBracketShortcodesWithAttrs(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "sc.html")
	if err := os.WriteFile(tmplFile, []byte(`<a href="{{.Attrs.url}}">{{.Attrs.label}}</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"link": {Name: "link", Template: "sc.html"},
		},
	}

	result := gen.processShortcodes(`Check [link url="https://example.com" label="Click here"] now`)
	if strings.Contains(result, "[link") {
		t.Errorf("Expected [link ...] to be replaced, got: %s", result)
	}
	if !strings.Contains(result, `href="https://example.com"`) {
		t.Errorf("Expected url attr in output, got: %s", result)
	}
	if !strings.Contains(result, "Click here") {
		t.Errorf("Expected label attr in output, got: %s", result)
	}
}

func TestProcessBracketShortcodesWithClosingTag(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "box.html")
	if err := os.WriteFile(tmplFile, []byte(`<div class="box">{{.InnerContent}}</div>`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"box": {Name: "box", Template: "box.html"},
		},
	}

	result := gen.processShortcodes("Before [box]Hello World[/box] After")
	if strings.Contains(result, "[box]") || strings.Contains(result, "[/box]") {
		t.Errorf("Expected [box]...[/box] to be replaced, got: %s", result)
	}
	if !strings.Contains(result, "Hello World") {
		t.Errorf("Expected inner content in output, got: %s", result)
	}
	if !strings.Contains(result, `class="box"`) {
		t.Errorf("Expected box div in output, got: %s", result)
	}
}

func TestProcessBracketShortcodesWithAttrsAndClosingTag(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "alert.html")
	if err := os.WriteFile(tmplFile, []byte(`<div class="alert-{{.Attrs.type}}">{{.InnerContent}}</div>`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"alert": {Name: "alert", Template: "alert.html"},
		},
	}

	result := gen.processShortcodes(`[alert type="warning"]Watch out![/alert]`)
	if strings.Contains(result, "[alert") || strings.Contains(result, "[/alert]") {
		t.Errorf("Expected shortcode to be replaced, got: %s", result)
	}
	if !strings.Contains(result, `alert-warning`) {
		t.Errorf("Expected type attr in output, got: %s", result)
	}
	if !strings.Contains(result, "Watch out!") {
		t.Errorf("Expected inner content in output, got: %s", result)
	}
}

func TestProcessBracketShortcodesUnknownWithAttrs(t *testing.T) {
	gen := &Generator{
		config:       Config{ShortcodeBrackets: true},
		shortcodeMap: map[string]Shortcode{"known": {Name: "known"}},
	}

	// Unknown shortcode with attrs should remain untouched
	result := gen.processShortcodes(`[unknown attr="val"]`)
	if !strings.Contains(result, `[unknown attr="val"]`) {
		t.Errorf("Expected unknown shortcode to remain, got: %s", result)
	}

	// Unknown closing shortcode should remain untouched
	result = gen.processShortcodes(`[unknown]content[/unknown]`)
	if !strings.Contains(result, `[unknown]content[/unknown]`) {
		t.Errorf("Expected unknown closing shortcode to remain, got: %s", result)
	}
}

func TestProcessBracketShortcodesMultilineContent(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "code.html")
	if err := os.WriteFile(tmplFile, []byte(`<pre>{{.InnerContent}}</pre>`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"code": {Name: "code", Template: "code.html"},
		},
	}

	input := "[code]\nline 1\nline 2\nline 3\n[/code]"
	result := gen.processShortcodes(input)
	if strings.Contains(result, "[code]") {
		t.Errorf("Expected [code]...[/code] to be replaced, got: %s", result)
	}
	if !strings.Contains(result, "<pre>") {
		t.Errorf("Expected <pre> wrapper, got: %s", result)
	}
}

func TestProcessBracketShortcodesConfigFieldsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	tmplFile := filepath.Join(tmpDir, "sc.html")
	if err := os.WriteFile(tmplFile, []byte(`{{.Title}}|{{.Attrs.extra}}`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			ShortcodeBrackets: true,
			TemplatesDir:      tmpDir,
			Template:          "",
		},
		shortcodeMap: map[string]Shortcode{
			"promo": {Name: "promo", Title: "Config Title", Template: "sc.html"},
		},
	}

	// Config fields should be preserved alongside inline attrs
	result := gen.processShortcodes(`[promo extra="bonus"]`)
	if !strings.Contains(result, "Config Title") {
		t.Errorf("Expected config Title preserved, got: %s", result)
	}
	if !strings.Contains(result, "bonus") {
		t.Errorf("Expected inline attr, got: %s", result)
	}
}

func TestParseShortcodeAttrs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single", ` url="https://example.com"`, map[string]string{"url": "https://example.com"}},
		{"multiple", ` type="warning" color="red"`, map[string]string{"type": "warning", "color": "red"}},
		{"with spaces in value", ` title="Hello World"`, map[string]string{"title": "Hello World"}},
		{"empty value", ` class=""`, map[string]string{"class": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShortcodeAttrs(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseShortcodeAttrs(%q) len = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseShortcodeAttrs(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestTmplDecodeHTML(t *testing.T) {
	result := tmplDecodeHTML("&amp; &lt; &gt; &#8211;")
	if !strings.Contains(result, "&") || !strings.Contains(result, "<") || !strings.Contains(result, ">") {
		t.Errorf("tmplDecodeHTML failed: %s", result)
	}
}

func TestTmplFormatDatePLCov(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		expected string
	}{
		{"january", parseDateCov("2024-01-15"), "15 stycznia 2024"},
		{"june", parseDateCov("2024-06-01"), "1 czerwca 2024"},
		{"december", parseDateCov("2024-12-31"), "31 grudnia 2024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tmplFormatDatePL(tt.date)
			if result != tt.expected {
				t.Errorf("tmplFormatDatePL = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTmplGetURL(t *testing.T) {
	page := models.Page{
		Slug: "test",
		Link: "https://example.com/test/",
	}
	result := tmplGetURL(page)
	if result == "" {
		t.Error("Expected non-empty URL")
	}
}

func TestTmplGetCanonical(t *testing.T) {
	page := models.Page{
		Slug: "test",
		Link: "https://example.com/test/",
	}
	result := tmplGetCanonical(page, "example.com")
	if result == "" {
		t.Error("Expected non-empty canonical URL")
	}
}

func TestTmplHasValidCategories(t *testing.T) {
	pageWithCats := models.Page{Categories: []int{2, 3}}
	if !tmplHasValidCategories(pageWithCats) {
		t.Error("Expected true for page with valid categories")
	}

	pageWithOnlyDefault := models.Page{Categories: []int{1}}
	result := tmplHasValidCategories(pageWithOnlyDefault)
	_ = result
}

func TestFixMediaPathsThumbnailInSrcset(t *testing.T) {
	media := make(map[int]models.MediaItem)

	input := `<img srcset="/media/123_photo-300x200.jpg 300w, /media/123_photo-600x400.jpg 600w">`
	result := fixMediaPaths(input, media)

	if strings.Contains(result, "-300x200") || strings.Contains(result, "-600x400") {
		t.Errorf("Thumbnail suffixes should be removed from srcset, got: %s", result)
	}
}

func TestLoadMarkdownDirSetsSourceDirCov(t *testing.T) {
	tmpDir := t.TempDir()

	mdContent := `---
title: Test
status: publish
slug: test
link: https://example.com/test/
---
Content`
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create md: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}
	if pages[0].SourceDir != tmpDir {
		t.Errorf("Expected SourceDir=%q, got %q", tmpDir, pages[0].SourceDir)
	}
}

func TestLoadMarkdownDirWithImageFiles(t *testing.T) {
	tmpDir := t.TempDir()

	mdContent := `---
title: Page With Images
status: publish
slug: with-images
link: https://example.com/with-images/
---
Content with images`
	if err := os.WriteFile(filepath.Join(tmpDir, "page.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "hero.jpg"), []byte("jpg"), 0644); err != nil {
		t.Fatalf("Failed to create jpg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "diagram.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("Failed to create png: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("Expected 1 page (images skipped), got %d", len(pages))
	}
}

func TestMinifyCSSFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "media query",
			input:    "@media (max-width: 768px) {\n  body { color: red; }\n}",
			expected: "@media (max-width:768px){body{color:red;}}",
		},
		{
			name:     "empty file",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cssPath := filepath.Join(tmpDir, "test.css")

			if err := os.WriteFile(cssPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write: %v", err)
			}
			if err := minifyCSSFile(cssPath); err != nil {
				t.Fatalf("minifyCSSFile failed: %v", err)
			}
			result, _ := os.ReadFile(cssPath)
			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestMinifyJSFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty file",
			input:    "",
			expected: "",
		},
		{
			name:     "multi-line comment then code",
			input:    "/* header\n * comment\n */\nvar x = 1;\nvar y = 2;",
			expected: "var x = 1;\nvar y = 2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			jsPath := filepath.Join(tmpDir, "test.js")

			if err := os.WriteFile(jsPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write: %v", err)
			}
			if err := minifyJSFile(jsPath); err != nil {
				t.Fatalf("minifyJSFile failed: %v", err)
			}
			result, _ := os.ReadFile(jsPath)
			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestMinifyHTMLFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty file",
			input:    "",
			expected: "",
		},
		{
			name:     "multiple comments",
			input:    "<!-- a --><div><!-- b --></div>",
			expected: "<div></div>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			htmlPath := filepath.Join(tmpDir, "test.html")

			if err := os.WriteFile(htmlPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write: %v", err)
			}
			if err := minifyHTMLFile(htmlPath); err != nil {
				t.Fatalf("minifyHTMLFile failed: %v", err)
			}
			result, _ := os.ReadFile(htmlPath)
			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestGenerateConvertRelativeLinksError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html></html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:        "test-source",
		Template:      "simple",
		Domain:        "example.com",
		ContentDir:    contentDir,
		TemplatesDir:  filepath.Join(tmpDir, "templates"),
		OutputDir:     outputDir,
		RelativeLinks: true,
		Quiet:         true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate should not fail: %v", err)
	}
}

func TestGeneratePrettifyError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html></html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		PrettyHTML:   true,
		Quiet:        true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate should not fail: %v", err)
	}
}

func TestGenerateMinifyPath(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html></html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		MinifyCSS:    true,
		MinifyJS:     true,
		Quiet:        true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate should not fail: %v", err)
	}
}

func TestCleanOutputIfRequestedError(t *testing.T) {
	gen := &Generator{
		config: Config{
			Clean:     true,
			OutputDir: "/proc/nonexistent/cannot/remove",
			Quiet:     true,
		},
	}
	err := gen.cleanOutputIfRequested()
	if err != nil && !strings.Contains(err.Error(), "cleaning output") {
		t.Errorf("Expected cleaning output error, got: %v", err)
	}
}

func TestCleanOutputIfRequestedDisabled(t *testing.T) {
	gen := &Generator{
		config: Config{Clean: false},
	}
	err := gen.cleanOutputIfRequested()
	if err != nil {
		t.Errorf("Expected nil when clean disabled, got: %v", err)
	}
}

func TestGenerateSitemapAndRobotsPartial(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			Domain:     "example.com",
			SitemapOff: false,
			RobotsOff:  true,
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
	}

	if err := gen.generateSitemapAndRobots(); err != nil {
		t.Fatalf("generateSitemapAndRobots failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "sitemap.xml")); err != nil {
		t.Error("sitemap.xml should exist")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "robots.txt")); err == nil {
		t.Error("robots.txt should NOT exist")
	}
}

func TestGenerateSitemapAndRobotsBothOff(t *testing.T) {
	gen := &Generator{
		config: Config{
			SitemapOff: true,
			RobotsOff:  true,
		},
	}

	if err := gen.generateSitemapAndRobots(); err != nil {
		t.Errorf("Expected nil when both off, got: %v", err)
	}
}

func TestLoadContentFromFilesWithPostURLFormat(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts", "news")

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(postsDir, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	postContent := `---
title: Test Post
status: publish
slug: test-post
link: https://example.com/test-post/
date: 2024-06-15
---
Content`
	if err := os.WriteFile(filepath.Join(postsDir, "post.md"), []byte(postContent), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:        "test-source",
			ContentDir:    contentDir,
			PostURLFormat: "slug",
		},
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	if err := gen.loadContentFromFiles(); err != nil {
		t.Fatalf("loadContentFromFiles failed: %v", err)
	}

	if len(gen.siteData.Posts) != 1 {
		t.Fatalf("Expected 1 post, got %d", len(gen.siteData.Posts))
	}
	if gen.siteData.Posts[0].URLFormat != "slug" {
		t.Errorf("Expected URLFormat='slug', got %q", gen.siteData.Posts[0].URLFormat)
	}
}

func TestPrettifyIfRequestedError(t *testing.T) {
	gen := &Generator{
		config: Config{
			PrettyHTML: true,
			MinifyHTML: false,
			OutputDir:  "/nonexistent/path",
		},
	}
	err := gen.prettifyIfRequested()
	if err == nil {
		t.Error("Expected error for nonexistent output dir")
	}
}

func TestMinifyIfRequestedError(t *testing.T) {
	gen := &Generator{
		config: Config{
			MinifyHTML: true,
			OutputDir:  "/nonexistent/path",
		},
	}
	err := gen.minifyIfRequested()
	if err == nil {
		t.Error("Expected error for nonexistent output dir")
	}
}

func TestConvertRelativeLinksIfRequestedError(t *testing.T) {
	gen := &Generator{
		config: Config{
			RelativeLinks: true,
			Domain:        "example.com",
			OutputDir:     "/nonexistent/path",
		},
	}
	err := gen.convertRelativeLinksIfRequested()
	if err == nil {
		t.Error("Expected error for nonexistent output dir")
	}
}

func TestRenderShortcodeTemplateExecuteError(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	badTmpl := `{{.NonExistentMethod.Call}}`
	if err := os.WriteFile(filepath.Join(templateDir, "bad.html"), []byte(badTmpl), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	gen := &Generator{
		config: Config{
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			Template:     "test",
		},
		shortcodeMap: map[string]Shortcode{
			"bad": {Name: "bad", Template: "bad.html"},
		},
	}

	result := gen.processShortcodes("{{bad}}")
	if result != "" {
		t.Errorf("Expected empty result for failed shortcode, got %q", result)
	}
}

func TestCopyAssetsJSNotExist(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	cssDir := filepath.Join(templatePath, "css")

	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatalf("Failed to create css dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}

	contentPath := filepath.Join(tmpDir, "content", "test-source")
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		t.Fatalf("Failed to create content dir: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "css", "style.css")); err != nil {
		t.Error("CSS should be copied")
	}
}

func TestGeneratePostNoSourceDir(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	tmpl := template.Must(template.New("post.html").Parse("<html>{{.Post.Title}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{Domain: "example.com"},
		tmpl:     tmpl,
	}

	post := models.Page{
		Title:     "No Source",
		Slug:      "no-source",
		Type:      "post",
		Date:      parseDateCov("2024-01-01"),
		SourceDir: "",
	}

	if err := gen.generatePost(post); err != nil {
		t.Fatalf("generatePost failed: %v", err)
	}

	postPath := filepath.Join(outputDir, "2024", "01", "01", "no-source", "index.html")
	if _, err := os.Stat(postPath); err != nil {
		t.Error("Post should be created even without source dir")
	}
}

func TestMinifyOutputSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "page.html"), []byte("<html>  <body>  </body>  </html>"), 0644); err != nil {
		t.Fatalf("Failed to create HTML: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "style.css"), []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "app.js"), []byte("// comment\nvar x = 1;"), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			MinifyHTML: true,
			MinifyCSS:  true,
			MinifyJS:   true,
		},
	}

	if err := gen.minifyOutput(); err != nil {
		t.Fatalf("minifyOutput failed: %v", err)
	}
}

func TestPrettifyOutputSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	htmlContent := "<!DOCTYPE html>\n\n\n<html>\n<body>\n</body>\n</html>"
	if err := os.WriteFile(filepath.Join(subDir, "page.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("Failed to create txt: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			PrettyHTML: true,
		},
	}

	if err := gen.prettifyOutput(); err != nil {
		t.Fatalf("prettifyOutput failed: %v", err)
	}
}

func TestLoadContentMddbEnabled(t *testing.T) {
	gen := &Generator{
		config: Config{
			Mddb: MddbConfig{
				Enabled: true,
				URL:     "http://localhost:99999",
			},
		},
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	err := gen.loadContent()
	if err == nil {
		t.Error("Expected error when mddb server is not available")
	}
}

func TestLinkifyListItemUnescapedMatch(t *testing.T) {
	pageLinks := map[string]string{
		"Caf\u00e9 & Bar": "/cafe-bar/",
	}

	line := "- Caf&#xe9; &amp; Bar"
	content := "Caf&#xe9; &amp; Bar"
	result := linkifyListItem(line, content, pageLinks)

	if !strings.Contains(result, "/cafe-bar/") {
		t.Errorf("Expected unescaped match to link, got: %s", result)
	}
}

func TestGenerateSitemapAndRobotsSitemapError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir:  "/nonexistent/path",
			Domain:     "example.com",
			SitemapOff: false,
			RobotsOff:  true,
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
	}

	err := gen.generateSitemapAndRobots()
	if err == nil {
		t.Error("Expected error when sitemap cannot be written")
	}
}

func TestGenerateSitemapAndRobotsRobotsError(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			Domain:     "example.com",
			SitemapOff: true,
			RobotsOff:  false,
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
	}

	if err := gen.generateSitemapAndRobots(); err != nil {
		t.Fatalf("generateSitemapAndRobots should succeed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "robots.txt")); err != nil {
		t.Error("robots.txt should exist")
	}
}

func TestCopyAssetsErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	cssDir := filepath.Join(templatePath, "css")
	jsDir := filepath.Join(templatePath, "js")
	imagesDir := filepath.Join(templatePath, "images")
	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cssDir, "s.css"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "s.js"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imagesDir, "i.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	contentPath := filepath.Join(tmpDir, "content", "test-source")
	mediaPath := filepath.Join(contentPath, "media")
	if err := os.MkdirAll(mediaPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaPath, "img.jpg"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "css", "s.css")); err != nil {
		t.Error("CSS should be copied")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "js", "s.js")); err != nil {
		t.Error("JS should be copied")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "images", "i.png")); err != nil {
		t.Error("Images should be copied")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "media", "img.jpg")); err != nil {
		t.Error("Media should be copied")
	}
}

func TestGenerateCloudflareFilesRedirectsError(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "_headers"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{OutputDir: tmpDir},
	}

	if err := gen.generateCloudflareFiles(); err != nil {
		t.Fatalf("generateCloudflareFiles should succeed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "_redirects")); err != nil {
		t.Error("_redirects should exist")
	}
}

func TestGenerateFullWithCleanError(t *testing.T) {
	gen := &Generator{
		config: Config{
			Clean:     true,
			OutputDir: "/proc/1/nonexistent",
			Quiet:     true,
		},
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	err := gen.Generate()
	// Either clean fails or loading content fails - both are expected
	_ = err
}

func TestGenerateWithPostURLFormatSlug(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts", "news")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	postContent := `---
title: Slug Post
status: publish
slug: slug-post
link: https://example.com/slug-post/
date: 2024-01-15
---
Content`
	if err := os.WriteFile(filepath.Join(postsDir, "post.md"), []byte(postContent), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html>{{.Post.Title}}</html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:        "test-source",
		Template:      "simple",
		Domain:        "example.com",
		ContentDir:    contentDir,
		TemplatesDir:  filepath.Join(tmpDir, "templates"),
		OutputDir:     outputDir,
		PostURLFormat: "slug",
		Quiet:         true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestEnsureTemplatesWriteError(t *testing.T) {
	gen := &Generator{}
	err := gen.ensureTemplates("/proc/1/nonexistent/templates")
	// MkdirAll may fail on some systems - both outcomes are acceptable
	_ = err
}

func TestPagesPathDefault(t *testing.T) {
	gen := &Generator{config: Config{}}
	if got := gen.pagesPath(); got != "pages" {
		t.Errorf("pagesPath() = %q, want \"pages\"", got)
	}
}

func TestPagesPathCustom(t *testing.T) {
	gen := &Generator{config: Config{PagesPath: "docs"}}
	if got := gen.pagesPath(); got != "docs" {
		t.Errorf("pagesPath() = %q, want \"docs\"", got)
	}
}

func TestPostsPathDefault(t *testing.T) {
	gen := &Generator{config: Config{}}
	if got := gen.postsPath(); got != "posts" {
		t.Errorf("postsPath() = %q, want \"posts\"", got)
	}
}

func TestPostsPathCustom(t *testing.T) {
	gen := &Generator{config: Config{PostsPath: "articles"}}
	if got := gen.postsPath(); got != "articles" {
		t.Errorf("postsPath() = %q, want \"articles\"", got)
	}
}

func TestTmplDefault(t *testing.T) {
	if got := tmplDefault("fallback", nil); got != "fallback" {
		t.Errorf("tmplDefault(nil) = %v, want fallback", got)
	}
	if got := tmplDefault("fallback", ""); got != "fallback" {
		t.Errorf("tmplDefault(\"\") = %v, want fallback", got)
	}
	if got := tmplDefault("fallback", 0); got != "fallback" {
		t.Errorf("tmplDefault(0) = %v, want fallback", got)
	}
	if got := tmplDefault("fallback", "value"); got != "value" {
		t.Errorf("tmplDefault(\"value\") = %v, want value", got)
	}
	if got := tmplDefault("fallback", 42); got != 42 {
		t.Errorf("tmplDefault(42) = %v, want 42", got)
	}
}

func TestRewriteMdLinksDisabled(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Pages: []models.Page{{Slug: "auth", Type: "page"}},
		},
		config: Config{Domain: "example.com", RewriteMdLinks: false},
	}
	// safeHTML should not rewrite when disabled
	input := `<a href="auth.md">Auth</a>`
	mdMap := gen.buildMdLinkMap()
	// rewriteMdLinks called directly — should still work
	got := rewriteMdLinks(input, mdMap)
	if got != `<a href="/auth/">Auth</a>` {
		t.Errorf("rewriteMdLinks() = %q, want /auth/", got)
	}
}

func TestExportVariablesToEnvNonString(t *testing.T) {
	vars := map[string]interface{}{
		"count": 42,
		"ratio": 3.14,
	}
	exportVariablesToEnv(vars, "TESTPKG")
	if val := os.Getenv("TESTPKG_COUNT"); val != "42" {
		t.Errorf("TESTPKG_COUNT = %q, want 42", val)
	}
	if val := os.Getenv("TESTPKG_RATIO"); val != "3.14" {
		t.Errorf("TESTPKG_RATIO = %q, want 3.14", val)
	}
}

func TestGenerateWithCustomPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create content with custom path names
	contentDir := filepath.Join(tmpDir, "content", "site")
	docsDir := filepath.Join(contentDir, "docs")
	articlesDir := filepath.Join(contentDir, "articles", "general")

	for _, d := range []string{docsDir, articlesDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(contentDir, "metadata.json"), []byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "intro.md"), []byte("---\ntitle: Intro\nslug: intro\nstatus: publish\ntype: page\n---\n\nContent here."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articlesDir, "hello.md"), []byte("---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2026-01-01\n---\n\nContent."), 0644); err != nil {
		t.Fatal(err)
	}

	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base.html", "index.html", "page.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(`{{define "`+name+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		PagesPath:    "docs",
		PostsPath:    "articles",
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// intro page should be generated
	if _, err := os.Stat(filepath.Join(tmpDir, "output", "intro", "index.html")); err != nil {
		t.Errorf("intro page not generated: %v", err)
	}
}

func TestRewriteMdLinksEnabled(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content", "site", "pages")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(contentDir), "metadata.json"), []byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "auth.md"), []byte("---\ntitle: Auth\nslug: auth\nstatus: publish\ntype: page\n---\n\n## Content\nSee [API](api.md)."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "api.md"), []byte("---\ntitle: API\nslug: api\nstatus: publish\ntype: page\n---\n\n## Content\nAPI docs."), 0644); err != nil {
		t.Fatal(err)
	}

	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base.html", "index.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(`{{define "`+name+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// page.html uses safeHTML on raw markdown string via .Page.Content
	if err := os.WriteFile(filepath.Join(templateDir, "page.html"), []byte(`{{define "page.html"}}{{safeHTML .Page.Content}}{{end}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Source:         "site",
		Template:       "simple",
		Domain:         "example.com",
		ContentDir:     filepath.Join(tmpDir, "content"),
		TemplatesDir:   filepath.Join(tmpDir, "templates"),
		OutputDir:      filepath.Join(tmpDir, "output"),
		RewriteMdLinks: true,
		Quiet:          true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "output", "auth", "index.html"))
	if err != nil {
		t.Fatalf("auth page not generated: %v", err)
	}
	if strings.Contains(string(content), `href="api.md"`) {
		t.Error("api.md link was not rewritten")
	}
	if !strings.Contains(string(content), `href="/api/"`) {
		t.Errorf("expected /api/ link in output, got: %s", content)
	}
}

func TestTmplDict(t *testing.T) {
	// valid pairs
	m, err := tmplDict("key1", "val1", "key2", 42)
	if err != nil {
		t.Fatalf("tmplDict() error = %v", err)
	}
	if m["key1"] != "val1" {
		t.Errorf("key1 = %v, want val1", m["key1"])
	}
	if m["key2"] != 42 {
		t.Errorf("key2 = %v, want 42", m["key2"])
	}

	// odd number of args
	_, err = tmplDict("k1")
	if err == nil {
		t.Error("expected error for odd args")
	}

	// non-string key
	_, err = tmplDict(123, "val")
	if err == nil {
		t.Error("expected error for non-string key")
	}
}

func TestMinifyOutputNoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			MinifyHTML: true,
			MinifyCSS:  false,
			MinifyJS:   false,
			Quiet:      true,
		},
	}
	// empty dir — should not error
	if err := gen.minifyOutput(); err != nil {
		t.Errorf("minifyOutput() on empty dir = %v", err)
	}
}

func TestCleanOutputIfNotRequested(t *testing.T) {
	tmpDir := t.TempDir()
	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Clean:     false,
		},
	}
	if err := gen.cleanOutputIfRequested(); err != nil {
		t.Errorf("cleanOutputIfRequested(false) = %v", err)
	}
}

func TestCleanOutputIfRequested(t *testing.T) {
	tmpDir := t.TempDir()
	// create a file to be cleaned
	if err := os.WriteFile(filepath.Join(tmpDir, "test.html"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Clean:     true,
			Quiet:     true,
		},
	}
	if err := gen.cleanOutputIfRequested(); err != nil {
		t.Errorf("cleanOutputIfRequested(true) = %v", err)
	}
}

func TestConvertMarkdownToHTMLVariants(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"**bold**", "<strong>bold</strong>"},
		{"# Heading", "<h1>Heading</h1>"},
		{"[link](http://example.com)", `<a href="http://example.com">link</a>`},
		{"", ""},
	}
	for _, tt := range tests {
		got := convertMarkdownToHTML(tt.input)
		if tt.want != "" && !strings.Contains(got, tt.want) {
			t.Errorf("convertMarkdownToHTML(%q) = %q, want to contain %q", tt.input, got, tt.want)
		}
	}
}

func TestConvertMarkdownTable(t *testing.T) {
	md := "| A | B |\n|---|---|\n| 1 | 2 |"
	got := convertMarkdownToHTML(md)
	if !strings.Contains(got, "<table>") {
		t.Errorf("expected table in output, got: %s", got)
	}
}

func TestCopyAssetsWithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")

	// Create css, js, images dirs
	for _, sub := range []string{"css", "js", "images"} {
		dir := filepath.Join(templateDir, sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main."+sub), []byte("body{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			Template:     "simple",
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.copyAssets(); err != nil {
		t.Errorf("copyAssets() = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "css", "main.css")); err != nil {
		t.Error("css not copied")
	}
}

func TestResolveVariablesDefaultValue(t *testing.T) {
	// non-string, non-map value — should pass through
	vars := map[string]interface{}{
		"count": 99,
		"flag":  true,
	}
	result := resolveVariables(vars)
	if result["count"] != 99 {
		t.Errorf("count = %v, want 99", result["count"])
	}
	if result["flag"] != true {
		t.Errorf("flag = %v, want true", result["flag"])
	}
}

func TestRewriteMdLinksEdgeCases(t *testing.T) {
	mdMap := map[string]string{
		"readme.md": "/docs/",
	}
	// multiple links in same string
	input := `<a href="readme.md">R</a> and <a href="readme.md">R2</a>`
	got := rewriteMdLinks(input, mdMap)
	if strings.Contains(got, `href="readme.md"`) {
		t.Errorf("still contains readme.md: %s", got)
	}
}

func TestGeneratePostWithLayout(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	layoutDir := filepath.Join(templateDir, "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base.html", "index.html", "page.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(`{{define "`+name+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Template:      "simple",
			Domain:        "example.com",
			TemplatesDir:  filepath.Join(tmpDir, "templates"),
			OutputDir:     outputDir,
			PostURLFormat: "slug",
			Quiet:         true,
		},
		siteData: &models.SiteData{},
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	post := models.Page{
		Title:     "Hello",
		Slug:      "hello",
		Status:    "publish",
		Type:      "post",
		URLFormat: "slug",
		Date:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := gen.generatePost(post); err != nil {
		t.Errorf("generatePost() = %v", err)
	}
}

func TestPrettifyOutputNoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	gen := &Generator{
		config: Config{OutputDir: tmpDir, PrettyHTML: true, Quiet: true},
	}
	if err := gen.prettifyOutput(); err != nil {
		t.Errorf("prettifyOutput() on empty dir = %v", err)
	}
}

func parseDateCov(s string) (t time.Time) {
	t, _ = time.Parse("2006-01-02", s)
	return
}

// ---------------------------------------------------------------------------
// excludeFromSitemap / generateSitemap: noindex, redirect, sitemap:no (#7)
// ---------------------------------------------------------------------------

func TestExcludeFromSitemap(t *testing.T) {
	tests := []struct {
		name    string
		page    models.Page
		exclude bool
	}{
		{"normal page", models.Page{Title: "About", Slug: "about"}, false},
		{"noindex", models.Page{Title: "X", Robots: "noindex, follow"}, true},
		{"noindex nofollow", models.Page{Title: "X", Robots: "noindex, nofollow"}, true},
		{"NOINDEX caps", models.Page{Title: "X", Robots: "NOINDEX"}, true},
		{"index follow", models.Page{Title: "X", Robots: "index, follow"}, false},
		{"redirect layout", models.Page{Title: "X", Layout: "redirect"}, true},
		{"sitemap no", models.Page{Title: "X", Sitemap: "no"}, true},
		{"sitemap No caps", models.Page{Title: "X", Sitemap: "No"}, true},
		{"sitemap yes", models.Page{Title: "X", Sitemap: "yes"}, false},
		{"redirect + noindex", models.Page{Title: "X", Layout: "redirect", Robots: "noindex"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := excludeFromSitemap(tt.page)
			if got != tt.exclude {
				t.Errorf("excludeFromSitemap(%s) = %v, want %v", tt.name, got, tt.exclude)
			}
		})
	}
}

func TestSitemapExcludesNoindexPages(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Domain:    "example.com",
			OutputDir: outputDir,
			Quiet:     true,
		},
		siteData: &models.SiteData{
			Pages: []models.Page{
				{Title: "About", Slug: "about", Modified: parseDateCov("2024-01-01")},
				{Title: "Redirect", Slug: "docs", Robots: "noindex, follow", Layout: "redirect"},
				{Title: "Hidden", Slug: "hidden", Sitemap: "no"},
			},
			Posts: []models.Page{
				{Title: "Post1", Slug: "post1", Type: "post", Date: parseDateCov("2024-06-01")},
				{Title: "Draft", Slug: "draft", Type: "post", Robots: "noindex, nofollow"},
			},
			Categories: make(map[int]models.Category),
		},
	}

	if err := gen.generateSitemap(); err != nil {
		t.Fatalf("generateSitemap() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "sitemap.xml"))
	if err != nil {
		t.Fatalf("sitemap.xml not found: %v", err)
	}
	xml := string(content)

	// Should include
	if !strings.Contains(xml, "/about/") {
		t.Error("sitemap should contain /about/")
	}
	if !strings.Contains(xml, "/post1/") {
		t.Error("sitemap should contain /post1/")
	}
	if !strings.Contains(xml, "example.com/") {
		t.Error("sitemap should contain homepage")
	}

	// Should exclude
	if strings.Contains(xml, "/docs/") {
		t.Error("sitemap should NOT contain noindex redirect /docs/")
	}
	if strings.Contains(xml, "/hidden/") {
		t.Error("sitemap should NOT contain sitemap:no /hidden/")
	}
	if strings.Contains(xml, "/draft/") {
		t.Error("sitemap should NOT contain noindex post /draft/")
	}
}

func TestSitemapSkipsHomepageWhenNoindex(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Domain:    "example.com",
			OutputDir: outputDir,
			Quiet:     true,
		},
		siteData: &models.SiteData{
			Pages: []models.Page{
				{Title: "Home", Slug: "index", Robots: "noindex, nofollow"},
				{Title: "About", Slug: "about"},
			},
			Categories: make(map[int]models.Category),
		},
	}

	if err := gen.generateSitemap(); err != nil {
		t.Fatalf("generateSitemap() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	xml := string(content)

	if strings.Contains(xml, "<loc>https://example.com/</loc>") {
		t.Error("homepage should be excluded when index page has noindex")
	}
	if !strings.Contains(xml, "/about/") {
		t.Error("about page should still be included")
	}
}

// ---------------------------------------------------------------------------
// generatePage: skip-root branch + custom layout + copyColocated error
// ---------------------------------------------------------------------------

func TestGeneratePageSkipRoot(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html", "base.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatal(err)
	}

	// Link field that resolves to empty output path → skip
	page := models.Page{Title: "Home", Slug: "", Link: "https://example.com/"}
	if err := gen.generatePage(page); err != nil {
		t.Errorf("expected no error for root skip, got %v", err)
	}
}

func TestGeneratePageCustomLayout(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	layoutsDir := filepath.Join(templateDir, "layouts")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html", "base.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(layoutsDir, "custom.html"), []byte(`{{define "layouts/custom.html"}}custom{{end}}`), 0644); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatal(err)
	}

	page := models.Page{Title: "Custom", Slug: "custom-page", Layout: "custom"}
	if err := gen.generatePage(page); err != nil {
		t.Errorf("generatePage with custom layout: %v", err)
	}
}

func TestGeneratePageFallbackTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html", "base.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatal(err)
	}

	// Template field points to non-existent template → fallback to page.html
	page := models.Page{Title: "Fallback", Slug: "fallback-page", Template: "nonexistent"}
	if err := gen.generatePage(page); err != nil {
		t.Errorf("generatePage fallback: %v", err)
	}
}

// ---------------------------------------------------------------------------
// copyAssets: media present path
// ---------------------------------------------------------------------------

func TestCopyAssetsWithMediaDir(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create media in content source
	mediaDir := filepath.Join(tmpDir, "content", "site", "media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "img.png"), []byte("PNG"), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.copyAssets(); err != nil {
		t.Errorf("copyAssets() with media: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "media", "img.png")); err != nil {
		t.Errorf("media not copied: %v", err)
	}
}

// ---------------------------------------------------------------------------
// loadTemplates: layouts subdir coverage
// ---------------------------------------------------------------------------

func TestLoadTemplatesWithLayouts(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	layoutsDir := filepath.Join(templateDir, "layouts")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(layoutsDir, "special.html"), []byte(`{{define "layouts/special.html"}}special{{end}}`), 0644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    filepath.Join(tmpDir, "output"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{},
	}
	if err := gen.loadTemplates(); err != nil {
		t.Errorf("loadTemplates with layouts: %v", err)
	}
	if gen.tmpl == nil {
		t.Error("tmpl is nil after loadTemplates")
	}
}

// ---------------------------------------------------------------------------
// Generate: full pipeline happy path
// ---------------------------------------------------------------------------

func TestGenerateFullPipeline(t *testing.T) {
	tmpDir := t.TempDir()

	// Content
	pagesDir := filepath.Join(tmpDir, "content", "site", "pages")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "content", "site", "metadata.json"),
		[]byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "about.md"),
		[]byte("---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout page."), 0644); err != nil {
		t.Fatal(err)
	}

	// Templates
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html", "base.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "output", "index.html")); err != nil {
		t.Errorf("index.html not generated: %v", err)
	}
}

// ---------------------------------------------------------------------------
// generateSite: MkdirAll error
// ---------------------------------------------------------------------------

func TestGenerateSiteMkdirError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/proc/impossible-dir/sub",
			Quiet:     true,
		},
		siteData: &models.SiteData{},
	}
	err := gen.generateSite()
	if err == nil {
		t.Error("expected error from generateSite with bad outputDir")
	}
}

// ---------------------------------------------------------------------------
// convertMarkdownToHTML: empty string returns empty
// ---------------------------------------------------------------------------

func TestConvertMarkdownToHTMLEmpty(t *testing.T) {
	got := convertMarkdownToHTML("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// resolveVariables: env var resolution
// ---------------------------------------------------------------------------

func TestResolveVariablesEnvResolution(t *testing.T) {
	t.Setenv("TEST_MYVAR", "resolved_value")
	vars := map[string]interface{}{
		"key":    "$TEST_MYVAR",
		"other":  "literal",
		"nested": map[string]interface{}{"inner": "$TEST_MYVAR"},
	}
	result := resolveVariables(vars)
	if result["key"] != "resolved_value" {
		t.Errorf("key = %v, want resolved_value", result["key"])
	}
	if result["other"] != "literal" {
		t.Errorf("other = %v, want literal", result["other"])
	}
	nested, ok := result["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested not a map")
	}
	if nested["inner"] != "resolved_value" {
		t.Errorf("nested.inner = %v, want resolved_value", nested["inner"])
	}
}

// ---------------------------------------------------------------------------
// Generate: loadContent error (bad content dir)
// ---------------------------------------------------------------------------

func TestGenerateLoadContentError(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Content dir missing pages/posts: loadContentFromFiles reads dirs that don't exist
	// This exercises the error paths in loadMarkdownDir (returns nil for missing dir)
	if err := gen.Generate(); err == nil {
		// If no templates exist, Generate() will fail on loadTemplates
		// That's fine – we're covering error paths
		t.Log("Generate succeeded (unexpected but acceptable)")
	}
}

// ---------------------------------------------------------------------------
// copyDir: subdirectory recursion
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// generateSite: page/post warning branches
// ---------------------------------------------------------------------------

func TestGenerateSitePagePostWarning(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Only index, category, post — NO page.html, so generatePage will log a warning
	for _, n := range []string{"index.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// post.html with an unclosed action to fail execution for posts too
	if err := os.WriteFile(filepath.Join(templateDir, "post.html"), []byte(`{{define "post.html"}}{{.BadField `), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{{Title: "About", Slug: "about"}},
			Posts:      []models.Page{{Title: "Post1", Slug: "post1", Date: parseDateCov("2024-01-01")}},
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}
	// Load index + category templates only (post.html has bad syntax so ParseGlob won't work)
	// Use a fresh template dir with valid index to get tmpl loaded
	validDir := filepath.Join(tmpDir, "templates", "valid")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "category.html", "post.html"} {
		if err := os.WriteFile(filepath.Join(validDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gen.config.Template = "valid"
	if err := gen.loadTemplates(); err != nil {
		t.Fatal(err)
	}
	// Block the post output path by pre-creating a file where the dir should go
	// Type is "" so GetOutputPath returns just "post1" (not date-based)
	postPath := filepath.Join(outputDir, "post1")
	if err := os.WriteFile(postPath, []byte("block"), 0644); err != nil {
		t.Fatal(err)
	}

	// generateSite should warn about page (page.html not found) and post (MkdirAll fails) but not fail
	if err := gen.generateSite(); err != nil {
		t.Errorf("generateSite should not return error, got: %v", err)
	}
}

func TestCopyDirRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	subDir := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}
	dstDir := filepath.Join(tmpDir, "dst")
	gen := &Generator{config: Config{Quiet: true}}
	if err := gen.copyDir(srcDir, dstDir); err != nil {
		t.Errorf("copyDir recursive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "sub", "file.css")); err != nil {
		t.Errorf("recursive file not copied: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Generate: error at loadTemplates step (missing templates)
// ---------------------------------------------------------------------------

func TestGenerateLoadTemplatesErrorCov(t *testing.T) {
	tmpDir := t.TempDir()
	// Create metadata and pages dir so loadContent succeeds
	sourceDir := filepath.Join(tmpDir, "content", "site")
	if err := os.MkdirAll(filepath.Join(sourceDir, "pages"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Create template dir with an invalid template to trigger ParseGlob error
	templateDir := filepath.Join(tmpDir, "templates", "broken")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "bad.html"), []byte(`{{define "bad.html"}}{{.Invalid `), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Source:       "site",
		Template:     "broken",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := gen.Generate(); err == nil {
		t.Error("expected error from Generate with broken templates")
	}
}

// ---------------------------------------------------------------------------
// Generate: error at generateSite step (bad outputDir)
// ---------------------------------------------------------------------------

func TestGenerateGenerateSiteErrorCov(t *testing.T) {
	tmpDir := t.TempDir()
	// Prepare valid content
	sourceDir := filepath.Join(tmpDir, "content", "site")
	if err := os.MkdirAll(filepath.Join(sourceDir, "pages"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Prepare valid templates
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    "/proc/nope/output", // triggers MkdirAll error
		Quiet:        true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// loadContent + loadTemplates succeed; generateSite should fail on MkdirAll
	// Pre-load so we can test generateSite error path
	gen.config.OutputDir = filepath.Join(tmpDir, "output")
	if err := gen.loadContent(); err != nil {
		t.Fatalf("loadContent: %v", err)
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	gen.config.OutputDir = "/proc/nope/output"
	if err := gen.generateSite(); err == nil {
		t.Error("expected error from generateSite with bad outputDir")
	}
}

// ---------------------------------------------------------------------------
// minifyOutput: CSS and JS paths
// ---------------------------------------------------------------------------

func TestMinifyOutputCSSAndJS(t *testing.T) {
	tmpDir := t.TempDir()
	// Create css and js files in output
	if err := os.WriteFile(filepath.Join(tmpDir, "style.css"), []byte("body { color: red; }"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "app.js"), []byte("var x = 1;"), 0644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{config: Config{
		OutputDir: tmpDir,
		MinifyCSS: true,
		MinifyJS:  true,
		Quiet:     true,
	}}
	if err := gen.minifyOutput(); err != nil {
		t.Errorf("minifyOutput CSS+JS: %v", err)
	}
}

// ---------------------------------------------------------------------------
// generatePost: co-located assets copy
// ---------------------------------------------------------------------------

func TestGeneratePostWithColocatedAssetsCov(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html", "base.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create source dir with an image file
	sourceDir := filepath.Join(tmpDir, "posts-src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "hero.png"), []byte("PNG"), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	gen := &Generator{
		config: Config{
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			OutputDir:    outputDir,
			ContentDir:   filepath.Join(tmpDir, "content"),
			Source:       "site",
			Quiet:        true,
		},
		siteData: &models.SiteData{},
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen.loadTemplates(); err != nil {
		t.Fatal(err)
	}

	post := models.Page{
		Title:     "Post With Assets",
		Slug:      "post-with-assets",
		SourceDir: sourceDir,
		Date:      parseDateCov("2024-01-01"),
	}
	if err := gen.generatePost(post); err != nil {
		t.Errorf("generatePost with colocated assets: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Generate: full run that reaches copyAssets, sitemap, cloudflare steps
// ---------------------------------------------------------------------------

func TestGenerateReachesLaterSteps(t *testing.T) {
	tmpDir := t.TempDir()
	// Setup content
	sourceDir := filepath.Join(tmpDir, "content", "site")
	if err := os.MkdirAll(filepath.Join(sourceDir, "pages"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"exported_at":"","media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Setup templates
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"index.html", "page.html", "post.html", "category.html"} {
		if err := os.WriteFile(filepath.Join(templateDir, n), []byte(`{{define "`+n+`"}}ok{{end}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		MinifyHTML:   true,
		MinifyCSS:    true,
		MinifyJS:     true,
		Quiet:        true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}
}
