package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Catalog struct{ Messages map[string]map[string]any }

func LoadCatalog(dir string, languages []LanguageConfig) (*Catalog, error) {
	c := &Catalog{Messages: make(map[string]map[string]any)}
	for _, lang := range languages {
		var data []byte
		var ext string
		for _, candidate := range []string{".yaml", ".yml", ".json"} {
			b, err := os.ReadFile(filepath.Join(dir, lang.Code+candidate)) // #nosec G304 -- configured local catalog
			if err == nil {
				data, ext = b, candidate
				break
			}
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading catalog %s: %w", lang.Code, err)
			}
		}
		if data == nil {
			c.Messages[lang.Code] = map[string]any{}
			continue
		}
		var messages map[string]any
		var err error
		if ext == ".json" {
			err = json.Unmarshal(data, &messages)
		} else {
			err = yaml.Unmarshal(data, &messages)
		}
		if err != nil {
			return nil, fmt.Errorf("parsing catalog %s: %w", lang.Code, err)
		}
		c.Messages[lang.Code] = messages
	}
	return c, nil
}

func (c *Catalog) Lookup(lang, key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	var cur any = c.Messages[lang]
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
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

var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

func Interpolate(message string, vars map[string]any) string {
	return placeholderRE.ReplaceAllStringFunc(message, func(token string) string {
		name := placeholderRE.FindStringSubmatch(token)[1]
		if value, ok := vars[name]; ok {
			return fmt.Sprint(value)
		}
		return token
	})
}
