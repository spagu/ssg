package generator

import (
	"fmt"
	"path"
	"strings"
	"time"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

func (g *Generator) translationValue(key string, vars ...map[string]any) (string, error) {
	lang := g.currentLang
	if lang == "" {
		lang = g.config.DefaultLanguage
	}
	chain := []string{lang}
	if g.config.I18n.DictionaryFallback {
		chain = append(chain, g.config.I18n.FallbackLanguages[lang]...)
	}
	for _, candidate := range chain {
		if value, ok := g.catalog.Lookup(candidate, key); ok {
			message, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("translation %q for %q is not a string", key, candidate)
			}
			if len(vars) > 0 {
				message = ssgi18n.Interpolate(message, vars[0])
			}
			return message, nil
		}
	}
	switch g.config.I18n.MissingTranslation {
	case "error":
		return "", fmt.Errorf("missing translation %q for language %q", key, lang)
	case "empty":
		return "", nil
	case "fallback":
		return key, nil
	default:
		fmt.Printf("   ⚠️  missing translation %q for language %q\n", key, lang)
		return key, nil
	}
}

func pageFromAny(value any) (models.Page, bool) {
	switch p := value.(type) {
	case models.Page:
		return p, true
	case *models.Page:
		if p != nil {
			return *p, true
		}
	}
	return models.Page{}, false
}

func (g *Generator) translationURL(lang string, value any) string {
	p, ok := pageFromAny(value)
	if !ok {
		return ""
	}
	for _, tr := range p.Translations {
		if tr.Lang == lang {
			return tr.URL
		}
	}
	for _, tr := range g.translationsFor(p) {
		if tr.Lang == lang {
			return tr.URL
		}
	}
	return ""
}

func (g *Generator) languageURL(lang string) string {
	prefix := ssgi18n.Prefix(lang, g.config.DefaultLanguage, g.config.I18n)
	if prefix == "" {
		return "/"
	}
	return "/" + path.Clean(prefix) + "/"
}

func (g *Generator) localizeDate(value time.Time, preset string) string {
	lang := g.currentLang
	loc := g.langLocs[lang]
	if loc == nil {
		loc = g.siteLoc
	}
	if loc != nil {
		value = value.In(loc)
	}
	formats := map[string]string{"short": "2006-01-02", "medium": "2 Jan 2006", "long": "2 January 2006", "full": "Monday, 2 January 2006"}
	format := formats[strings.ToLower(preset)]
	if format == "" {
		format = formats["medium"]
	}
	return value.Format(format)
}

func languagePages(pages []models.Page, lang string) []models.Page {
	out := make([]models.Page, 0)
	for _, p := range pages {
		if p.Lang == lang {
			out = append(out, p)
		}
	}
	return out
}
