package externalsource

// Records into pages, one page per record (GO-098).
//
// The pieces were all here and did not meet. Seven input formats already parse
// into records; a CMS import already merges into the site through one path that
// gives imported documents the same URLs, taxonomies and outputs as native
// content; and `mode: content` already passed configuration validation for
// every source type. It just did nothing unless the source was a CMS database,
// so a CSV of products or a JSON API was data a template could iterate and
// never pages a site could have.
//
// What was missing is the only part a tool cannot guess: which field of a
// record is the title, and which is the body. `content_map` says so, and the
// records become pages on the path the CMS import already uses.

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// ContentMap names the record field behind each page field.
//
// A value is read as a dotted path into the record ("attributes.title"), unless
// it starts with "=" — then the rest is a constant, which is how every record
// gets `type: "=product"` — or contains "{{", making it a template over the
// record, which is how a slug is composed from more than one field.
type ContentMap struct {
	Title    string `yaml:"title" toml:"title" json:"title"`
	Slug     string `yaml:"slug" toml:"slug" json:"slug"`
	Content  string `yaml:"content" toml:"content" json:"content"`
	Excerpt  string `yaml:"excerpt" toml:"excerpt" json:"excerpt"`
	Date     string `yaml:"date" toml:"date" json:"date"`
	Modified string `yaml:"modified" toml:"modified" json:"modified"`
	Type     string `yaml:"type" toml:"type" json:"type"`
	Status   string `yaml:"status" toml:"status" json:"status"`
	Link     string `yaml:"link" toml:"link" json:"link"`
	// Description and Image feed the SEO pass exactly as frontmatter would.
	Description string `yaml:"description" toml:"description" json:"description"`
	Image       string `yaml:"image" toml:"image" json:"image"`
	Lang        string `yaml:"lang" toml:"lang" json:"lang"`
	// Taxonomies maps a taxonomy name to the record field holding its terms;
	// the field may be a list or a single value.
	Taxonomies map[string]string `yaml:"taxonomies" toml:"taxonomies" json:"taxonomies"`
}

// Empty reports whether nothing was mapped at all.
func (m ContentMap) Empty() bool {
	return m.Title == "" && m.Slug == "" && m.Content == "" && m.Excerpt == "" &&
		m.Date == "" && m.Modified == "" && m.Type == "" && m.Status == "" &&
		m.Link == "" && m.Description == "" && m.Image == "" && m.Lang == "" &&
		len(m.Taxonomies) == 0
}

// Content formats a mapped body can be declared as.
//
// Markdown is the default and the honest description of what happens: the body
// joins the same renderer every page's body goes through, which renders
// Markdown and passes HTML straight out — so `html` is accepted for a config
// that wants to say what it holds, and behaves the same.
//
// `text` is the one that changes something. Prose from an API is not Markdown,
// and rendering it as Markdown silently eats its asterisks, underscores and any
// line that happens to start with a "#". `text` escapes those first, so a
// product description arrives as its author wrote it.
const (
	FormatMarkdown = "markdown"
	FormatHTML     = "html"
	FormatText     = "text"
)

// MapRecords turns a loaded source's records into an import the generator
// merges like any CMS import.
func MapRecords(src Source, data interface{}) (*CMSImportResult, []string, error) {
	records, err := recordsOf(data)
	if err != nil {
		return nil, nil, err
	}
	limit := src.MaxRows
	if limit <= 0 {
		limit = defaultMaxRows
	}
	if len(records) > limit {
		return nil, nil, fmt.Errorf("%d records exceed max_rows (%d) — raise it or narrow the source", len(records), limit)
	}

	result := &CMSImportResult{Taxonomies: map[string][]string{}}
	seen := make(map[string]int, len(records))
	terms := map[string]map[string]bool{}
	var warnings []string
	strict := src.ContentErrors == "strict"

	for i, rec := range records {
		page, warns, err := mapRecord(src, rec, i)
		for _, w := range warns {
			if strict {
				return nil, nil, fmt.Errorf("record %d: %s", i+1, w)
			}
			warnings = append(warnings, fmt.Sprintf("source %q record %d: %s", src.Name, i+1, w))
		}
		if err != nil {
			if strict {
				return nil, nil, fmt.Errorf("record %d: %w", i+1, err)
			}
			warnings = append(warnings, fmt.Sprintf("source %q record %d skipped: %v", src.Name, i+1, err))
			continue
		}
		if first, dup := seen[page.Slug]; dup {
			return nil, nil, fmt.Errorf("records %d and %d both map to the slug %q — make the slug unique (a template like \"{{.sku}}-{{.name}}\" usually does it)", first+1, i+1, page.Slug)
		}
		seen[page.Slug] = i
		collectTerms(terms, page)
		if page.Type == "page" {
			result.Pages = append(result.Pages, *page)
		} else {
			result.Posts = append(result.Posts, *page)
		}
	}
	result.Taxonomies = sortedTerms(terms)
	return result, warnings, nil
}

// mapRecord builds one page. A record missing a mapped field is reported and
// the page is built without it; a record with no title has nothing to be a page
// about, so it is an error the caller's policy decides on.
func mapRecord(src Source, rec map[string]interface{}, i int) (*models.Page, []string, error) {
	m := src.ContentMap
	var warnings []string
	get := func(field, spec string) string {
		if spec == "" {
			return ""
		}
		v, err := resolveMapping(rec, spec)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("content_map.%s: %v", field, err))
			return ""
		}
		return v
	}

	title := get("title", m.Title)
	if title == "" {
		return nil, warnings, fmt.Errorf("no title (content_map.title = %q)", m.Title)
	}
	slug := get("slug", m.Slug)
	if slug == "" {
		slug = models.Slugify(title)
	} else {
		slug = models.Slugify(slug)
	}
	if slug == "" {
		return nil, warnings, fmt.Errorf("the slug is empty after normalisation (title %q)", title)
	}

	kind := get("type", m.Type)
	if kind == "" {
		kind = "post"
	}
	status := get("status", m.Status)
	if status == "" {
		status = "publish"
	}
	page := &models.Page{
		ID:            recordID(src.Name, i),
		Title:         title,
		Slug:          slug,
		Type:          kind,
		Status:        status,
		Content:       formatBody(get("content", m.Content), src.ContentFormat),
		Excerpt:       get("excerpt", m.Excerpt),
		Description:   get("description", m.Description),
		FeaturedImage: get("image", m.Image),
		Lang:          get("lang", m.Lang),
		Link:          get("link", m.Link),
	}
	if raw := get("date", m.Date); raw != "" {
		if t, ok := parseRecordTime(raw); ok {
			page.Date = t
		} else {
			warnings = append(warnings, fmt.Sprintf("content_map.date: %q is not a date this build recognises", raw))
		}
	}
	if raw := get("modified", m.Modified); raw != "" {
		if t, ok := parseRecordTime(raw); ok {
			page.Modified = t
		}
	}
	applyMappedTaxonomies(page, rec, m.Taxonomies, &warnings)
	return page, warnings, nil
}

// applyMappedTaxonomies fills the generic taxonomy map, plus the legacy tags
// and category fields, so mapped records reach every part of the site that
// still reads those.
func applyMappedTaxonomies(page *models.Page, rec map[string]interface{}, mapping map[string]string, warnings *[]string) {
	if len(mapping) == 0 {
		return
	}
	names := make([]string, 0, len(mapping))
	for name := range mapping {
		names = append(names, name)
	}
	sort.Strings(names)
	page.TaxonomiesFM = map[string]interface{}{}
	for _, name := range names {
		values, err := resolveMappingList(rec, mapping[name])
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("content_map.taxonomies.%s: %v", name, err))
			continue
		}
		if len(values) == 0 {
			continue
		}
		// []interface{} is the shape the frontmatter parser and the CMS
		// adapters both produce; the resolver reads one shape, not two.
		generic := make([]interface{}, 0, len(values))
		for _, v := range values {
			generic = append(generic, v)
		}
		page.TaxonomiesFM[name] = generic
		// The legacy fields as well, exactly as the WordPress adapter fills
		// them: the tag and category archives are still driven by these.
		switch name {
		case "tag", "tags":
			page.Tags = append(page.Tags, values...)
		case "category", "categories":
			if page.Category == "" {
				page.Category = values[0]
			}
			for _, v := range values {
				page.CategoriesRaw = append(page.CategoriesRaw, v)
			}
		}
	}
}

// collectTerms gathers every term a mapped page carries, so the site-level
// taxonomy list matches what a CMS import would have produced.
func collectTerms(into map[string]map[string]bool, page *models.Page) {
	for name, raw := range page.TaxonomiesFM {
		values, ok := raw.([]interface{})
		if !ok {
			continue
		}
		if into[name] == nil {
			into[name] = map[string]bool{}
		}
		for _, v := range values {
			if s, ok := v.(string); ok {
				into[name][s] = true
			}
		}
	}
}

// sortedTerms flattens the term set deterministically.
func sortedTerms(in map[string]map[string]bool) map[string][]string {
	out := make(map[string][]string, len(in))
	for name, set := range in {
		values := make([]string, 0, len(set))
		for v := range set {
			values = append(values, v)
		}
		sort.Strings(values)
		out[name] = values
	}
	return out
}

// formatBody prepares a mapped body for the renderer every page body goes
// through. See the format constants for why only `text` transforms anything.
func formatBody(body, format string) string {
	if format != FormatText {
		return body
	}
	return escapeMarkdown(body)
}

// markdownEscapes are the characters that turn prose into markup.
var markdownEscapes = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
	"<", "&lt;", ">", "&gt;", "#", `\#`, "|", `\|`,
)

// escapeMarkdown renders plain prose as itself.
func escapeMarkdown(s string) string { return markdownEscapes.Replace(s) }

// recordID gives every mapped page a stable id derived from the source name
// and its position, so two sources cannot collide and a rebuild is stable.
func recordID(name string, i int) int {
	h := 0
	for _, r := range name {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return (h%100000)*10000 + i + 1
}

// recordsOf coerces parsed data into a list of records. A source that parsed
// to one object is one record; a mapping of objects is its values, in key
// order, so a rebuild is stable.
func recordsOf(data interface{}) ([]map[string]interface{}, error) {
	switch v := data.(type) {
	case []map[string]interface{}:
		return v, nil
	case []map[string]string:
		out := make([]map[string]interface{}, 0, len(v))
		for _, row := range v {
			rec := make(map[string]interface{}, len(row))
			for k, val := range row {
				rec[k] = val
			}
			out = append(out, rec)
		}
		return out, nil
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for i, item := range v {
			rec, ok := toRecord(item)
			if !ok {
				return nil, fmt.Errorf("entry %d is %T, not an object — `transform.select` may be pointing at the wrong field", i+1, item)
			}
			out = append(out, rec)
		}
		return out, nil
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			if rec, ok := toRecord(v[k]); ok {
				out = append(out, rec)
			}
		}
		if len(out) == 0 {
			return []map[string]interface{}{v}, nil // one object is one record
		}
		return out, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("the source parsed to %T, which holds no records", data)
}

// toRecord accepts the two shapes a decoded object arrives in.
func toRecord(v interface{}) (map[string]interface{}, bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		return t, true
	case map[string]string:
		rec := make(map[string]interface{}, len(t))
		for k, val := range t {
			rec[k] = val
		}
		return rec, true
	}
	return nil, false
}

// resolveMapping reads one mapping value: a constant, a template, or a field.
func resolveMapping(rec map[string]interface{}, spec string) (string, error) {
	if constant, ok := strings.CutPrefix(spec, "="); ok {
		return constant, nil
	}
	if strings.Contains(spec, "{{") {
		return renderRecordTemplate(rec, spec)
	}
	v, ok := lookupField(rec, spec)
	if !ok {
		return "", fmt.Errorf("the record has no field %q (write %q to use it as a constant)", spec, "="+spec)
	}
	return scalarString(v), nil
}

// resolveMappingList reads a mapping value that may name several terms.
func resolveMappingList(rec map[string]interface{}, spec string) ([]string, error) {
	if constant, ok := strings.CutPrefix(spec, "="); ok {
		return splitTerms(constant), nil
	}
	if strings.Contains(spec, "{{") {
		rendered, err := renderRecordTemplate(rec, spec)
		if err != nil {
			return nil, err
		}
		return splitTerms(rendered), nil
	}
	v, ok := lookupField(rec, spec)
	if !ok {
		return nil, fmt.Errorf("the record has no field %q", spec)
	}
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := scalarString(item); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []string:
		return t, nil
	}
	return splitTerms(scalarString(v)), nil
}

// splitTerms accepts the comma-separated form a CSV column has to use.
func splitTerms(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// lookupField walks a dotted path into a record.
func lookupField(rec map[string]interface{}, path string) (interface{}, bool) {
	var cur interface{} = rec
	for _, part := range strings.Split(path, ".") {
		m, ok := toRecord(cur)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// renderRecordTemplate composes a value from several fields.
func renderRecordTemplate(rec map[string]interface{}, spec string) (string, error) {
	tmpl, err := template.New("map").Option("missingkey=zero").Parse(spec)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid template: %w", spec, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, rec); err != nil {
		return "", fmt.Errorf("%q: %w", spec, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// scalarString renders a record value as text without Go's %v decorations for
// the types that have a better spelling.
func scalarString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case time.Time:
		return t.Format(time.RFC3339)
	}
	return fmt.Sprintf("%v", v)
}

// recordTimeLayouts are the date spellings an API is likely to use.
var recordTimeLayouts = []string{
	time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02",
	"02/01/2006", time.RFC1123Z, time.RFC1123,
}

// parseRecordTime reads a date from a record, reporting whether it could.
func parseRecordTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range recordTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	// A Unix timestamp, which is what a database-backed API usually sends.
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}
