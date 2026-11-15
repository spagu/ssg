package config

// The text side of the configuration schema: how entries are written, the
// shapes of types that decode from more than one form, and the form groups.
// The entries themselves are in schema_docs_<area>.go, keyed by the Go type
// that owns the field, so a type used in two places (worker and workers[])
// is described once.

import (
	"reflect"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// keyDoc is one key's entry.
type keyDoc struct {
	class  string
	group  string // top-level keys only; nested keys inherit their parent's
	desc   string
	enum   []string
	alt    reflect.Type // a second accepted form
	secret string       // secret format; SecretWhole when empty
	mcp    bool         // writable through `ssg mcp` designer_config_set
	unset  any          // the value the code uses when the key is absent (see defaults)
}

// p, x and s start an entry of class plain, executes and secret.
func p(desc string) keyDoc { return keyDoc{class: ClassPlain, desc: desc} }
func x(desc string) keyDoc { return keyDoc{class: ClassExecutes, desc: desc} }
func s(desc string) keyDoc { return keyDoc{class: ClassSecret, desc: desc} }

// in places a top-level key in a form group.
func (d keyDoc) in(group string) keyDoc { d.group = group; return d }

// oneOf makes a string key an enum. "" in the list means "unset / off".
func (d keyDoc) oneOf(values ...string) keyDoc { d.enum = values; return d }

// or adds a second accepted form, given as a value of its type.
func (d keyDoc) or(sample any) keyDoc { d.alt = reflect.TypeOf(sample); return d }

// secretAs sets the secret format.
func (d keyDoc) secretAs(format string) keyDoc { d.secret = format; return d }

// defaults records what the code does when the key is absent, for a key whose
// zero value is not that behaviour (a *bool read as "on unless set to false").
// DefaultConfig cannot show it — the field stays nil — so without this the
// schema would leave such a switch without a default and a panel could only
// say "not set".
func (d keyDoc) defaults(v any) keyDoc { d.unset = v; return d }

// assistant marks a key writable through `ssg mcp` designer_config_set.
func (d keyDoc) assistant() keyDoc { d.mcp = true; return d }

// typeDocs maps a yaml key of one type to its entry.
type typeDocs map[string]keyDoc

// schemaDocs maps a Go type name ("config.Config") to its entries.
var schemaDocs = map[string]typeDocs{}

// schemaDocDuplicates lists keys registered twice; the schema test fails on any.
var schemaDocDuplicates []string

// describe registers the entries of the type of sample. Called once per file
// and type from a package-level var, so the table is complete before main.
func describe(sample any, docs typeDocs) bool {
	owner := reflect.TypeOf(sample).String()
	if schemaDocs[owner] == nil {
		schemaDocs[owner] = typeDocs{}
	}
	for k, d := range docs {
		if _, dup := schemaDocs[owner][k]; dup {
			schemaDocDuplicates = append(schemaDocDuplicates, "documented twice: "+owner+"."+k)
		}
		schemaDocs[owner][k] = d
	}
	return true
}

// schemaShape is what a type with its own decoder accepts.
type schemaShape struct {
	t    reflect.Type // the full form
	alt  reflect.Type // the short form, if any
	open bool         // the object also takes keys it does not name
	enum []string
}

// storyShape is `story:` as written: enabled, sections and rules mean
// something to the build, every other key goes to the page script.
type storyShape struct {
	Enabled  bool             `yaml:"enabled"`
	Sections []string         `yaml:"sections"`
	Rules    []storyRuleShape `yaml:"rules"`
}

// storyRuleShape is one `story.rules` entry.
type storyRuleShape struct {
	When storyWhenShape `yaml:"when"`
	Set  map[string]any `yaml:"set"`
}

// storyWhenShape is `story.rules[].when`, aliases included.
type storyWhenShape struct {
	Category   []string `yaml:"category"`
	Categories []string `yaml:"categories"`
	Tags       []string `yaml:"tags"`
	Tag        []string `yaml:"tag"`
	Section    []string `yaml:"section"`
	Sections   []string `yaml:"sections"`
	Contains   string   `yaml:"contains"`
}

// schemaShapes maps a type with a custom decoder to the shape it decodes.
var schemaShapes = map[reflect.Type]schemaShape{
	reflect.TypeOf(HeadingAnchorsConfig{}): {t: reflect.TypeOf(headingAnchorsFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(OGCardsConfig{}):        {t: reflect.TypeOf(ogCardsFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(WebPushConfig{}):        {t: reflect.TypeOf(webPushFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(CookModeConfig{}):       {t: reflect.TypeOf(cookModeFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(WebStoriesConfig{}):     {t: reflect.TypeOf(webStoriesFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(BreadcrumbsConfig{}):    {t: reflect.TypeOf(breadcrumbsFields{}), alt: reflect.TypeOf(true)},
	reflect.TypeOf(StoryConfig{}):          {t: reflect.TypeOf(storyShape{}), alt: reflect.TypeOf(true), open: true},
	reflect.TypeOf(OutputsSpec{}):          {t: reflect.TypeOf([]string{}), alt: reflect.TypeOf(map[string][]string{})},
	reflect.TypeOf(models.PathList{}):      {t: reflect.TypeOf([]string{}), alt: reflect.TypeOf("")},
	reflect.TypeOf(models.PrettyURLMode("")): {t: reflect.TypeOf(""), alt: reflect.TypeOf(true),
		enum: []string{"off", "strip", "strip-slash"}},
}

// languageConfigSample is the expanded `languages:` form, for .or().
var languageConfigSample = []ssgi18n.LanguageConfig{}

// Form group ids.
const (
	gSite       = "site"
	gContent    = "content"
	gURLs       = "urls"
	gLanguages  = "languages"
	gAppearance = "appearance"
	gPageTools  = "page-tools"
	gImages     = "images"
	gSEO        = "seo"
	gFeeds      = "feeds"
	gChecks     = "checks"
	gOutput     = "output"
	gBuild      = "build"
	gServer     = "server"
	gDeploy     = "deploy"
	gWorkers    = "workers"
	gSources    = "sources"
	gIntegr     = "integrations"
)

// schemaGroups are the form sections, in display order.
var schemaGroups = []SchemaGroup{
	{gSite, "Site", "the site's name, address, palette and social identity"},
	{gContent, "Content", "where content comes from and how Markdown is read"},
	{gURLs, "URLs and links", "permalinks, pretty URLs, redirects and link rewriting"},
	{gLanguages, "Languages and time", "languages, translations and time zones"},
	{gAppearance, "Appearance", "theme, code highlighting, diagrams, math and shortcodes"},
	{gPageTools, "Page tools", "story mode, Listen, Print, Cook Mode, share cards and other page features"},
	{gImages, "Images", "WebP, AVIF, responsive sizes and image metadata"},
	{gSEO, "SEO and discovery", "sitemaps, robots.txt, structured data, analytics and files for agents"},
	{gFeeds, "Feeds", "Atom/RSS/JSON feeds, product feeds and aggregation"},
	{gChecks, "Checks", "what the build validates after it writes the site"},
	{gOutput, "Output", "minification, assets, output formats, headers and archives"},
	{gBuild, "Build", "build behaviour, caches, profiling, hooks and limits"},
	{gServer, "Server", "the built-in development and preview server"},
	{gDeploy, "Deployment", "where and how the site is deployed"},
	{gWorkers, "Workers and modules", "Cloudflare Workers, endpoints, modules and the admin look"},
	{gSources, "Data sources", "MDDB, external sources and data files"},
	{gIntegr, "Integrations", "AI, text to speech, notifications and the MCP server"},
}
