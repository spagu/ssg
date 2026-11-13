package generator

// A theme that looks up parsed front matter in `.Extra` (#416).
//
// `.Extra` holds only the front matter the parser does not read itself. Since
// 1.8.85 (#378) that rule is enforced for every PageFrontmatter key, so
// `schema` and `alias_stubs` left `.Extra` for `.Schema` and `.AliasStubs`.
// A template that still says `index .Post.Extra "schema"` gets nil: no error,
// the fallback branch renders, the build passes. The reporter's in-page video
// player turned into a plain link on 86 posts and nobody noticed for two
// weeks. The lookup can never succeed, so the build says so and names the
// field that holds the value now.

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/parser"
)

// extraIndexRe matches `index <expr>.Extra "key"`.
var extraIndexRe = regexp.MustCompile(`index\s+((?:\$\w*)?(?:\.\w+)*\.Extra)\s+"([^"]+)"`)

// extraFieldRe matches `<expr>.Extra.key`.
var extraFieldRe = regexp.MustCompile(`((?:\$\w*)?(?:\.\w+)*\.Extra)\.(\w+)`)

// pageFieldByKey maps a front-matter key to the models.Page field that holds
// it, read from the struct's yaml tags so it cannot drift from the model.
var pageFieldByKey = func() map[string]string {
	fields := map[string]string{}
	t := reflect.TypeOf(models.Page{})
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ","); name != "" && name != "-" {
			fields[name] = t.Field(i).Name
		}
	}
	return fields
}()

// extraKeyFinding is one `.Extra` lookup of a key the parser keeps elsewhere.
type extraKeyFinding struct {
	File  string
	Line  int
	Expr  string // the map expression, e.g. ".Post.Extra"
	Key   string
	Field string // the models.Page field holding the key
}

// Message is the line a build prints.
func (f extraKeyFinding) Message() string {
	use := strings.TrimSuffix(f.Expr, "Extra") + f.Field
	return fmt.Sprintf("%s:%d: %s looks up %q, but the parser reads that front matter into its own field "+
		"and .Extra no longer holds it (since 1.8.85, #378) — the lookup is always empty; use %s instead.",
		f.File, f.Line, f.Expr, f.Key, use)
}

// checkTemplateExtraKeys reports every `.Extra` lookup in one theme file of a
// key that left `.Extra` in 1.8.85. Other keys are left alone: custom front
// matter is what `.Extra` is for, and an external record keeps every field
// there, `title` included.
func checkTemplateExtraKeys(file, src string) []extraKeyFinding {
	var findings []extraKeyFinding
	for _, re := range []*regexp.Regexp{extraIndexRe, extraFieldRe} {
		for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
			key := src[m[4]:m[5]]
			if !parser.LeftExtra(key) {
				continue
			}
			findings = append(findings, extraKeyFinding{
				File:  file,
				Line:  lineOf(src, m[0]),
				Expr:  src[m[2]:m[3]],
				Key:   key,
				Field: pageFieldByKey[key],
			})
		}
	}
	return findings
}
