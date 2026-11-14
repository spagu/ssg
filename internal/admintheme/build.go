package admintheme

import (
	"fmt"
	"path"
	"strings"

	"github.com/spagu/ssg/internal/modules"
)

// Finding is a problem the build reports under check_admin_theme: the key it
// is about and what is wrong. Configuration errors are returned as errors
// instead — a value that cannot be used at all never becomes a warning.
type Finding struct {
	Key    string
	Detail string
}

// Result is a computed theme: the registry's `theme`, the theme.css to
// write (nil: keep the shipped empty file) and the findings.
type Result struct {
	Theme    *modules.Theme
	CSS      []byte
	Findings []Finding
}

// Build validates c and computes the theme. file names the configuration in
// errors; exists reports whether a site path is in the build output (a logo
// or font that is not is a finding: the panels would ask for a file that
// is not there). A zero Config is no theme at all.
func Build(file string, c Config, exists func(string) bool) (Result, error) {
	if isZero(c) {
		return Result{}, nil
	}
	t := &modules.Theme{FontFiles: []modules.FontFile{}, Derived: []string{}, Light: map[string]string{}, Dark: map[string]string{},
		Adjustments: []modules.Adjustment{}, Checks: []modules.Check{}}
	var res Result
	if c.Name != "" {
		if err := checkName(file, c.Name); err != nil {
			return Result{}, err
		}
		name := strings.TrimSpace(c.Name)
		t.Name = &name
	}
	present := func(key, p string) {
		if !exists(p) {
			res.Findings = append(res.Findings, Finding{Key: key, Detail: fmt.Sprintf("%s is not in the build output, so the panels will not find it — add it to static/ (or correct the path)", p)})
		}
	}
	for _, l := range []struct {
		key, val string
		dst      **string
	}{{"logo", c.Logo, &t.Logo}, {"logo_dark", c.LogoDark, &t.LogoDark}} {
		if l.val == "" {
			continue
		}
		if err := checkFile(file, l.key, l.val, logoExt, "an image the panels can show (.svg, .png or .webp)"); err != nil {
			return Result{}, err
		}
		v := l.val
		*l.dst = &v
		present(l.key, v)
	}
	if err := fonts(file, c, t, present); err != nil {
		return Result{}, err
	}
	findings, err := colours(file, c, t)
	if err != nil {
		return Result{}, err
	}
	res.Findings = append(res.Findings, findings...)
	res.Theme, res.CSS = t, CSS(t)
	return res, nil
}

func isZero(c Config) bool {
	return c.Name == "" && c.Logo == "" && c.LogoDark == "" && c.Main == "" && c.Accent == "" && c.Font == "" && c.FontBody == "" &&
		len(c.FontFiles) == 0 && len(c.Tokens) == 0
}

// fonts validates the two stacks and the font files.
func fonts(file string, c Config, t *modules.Theme, present func(key, p string)) error {
	for _, f := range []struct {
		key, val, generic string
		dst               **string
	}{{"font", c.Font, "serif", &t.FontDisplay}, {"font_body", c.FontBody, "sans-serif", &t.FontUI}} {
		if f.val == "" {
			continue
		}
		s, err := Stack(f.val, f.generic)
		if err != nil {
			return configError(file, f.key, "%v", err)
		}
		*f.dst = &s
	}
	if len(c.FontFiles) > maxFontFiles {
		return configError(file, "font_files", "lists %d files; %d at most", len(c.FontFiles), maxFontFiles)
	}
	for i, f := range c.FontFiles {
		key := fmt.Sprintf("font_files[%d]", i)
		family, err := FamilyName(f.Family)
		if err != nil {
			return configError(file, key+".family", "%v", err)
		}
		if err := checkFile(file, key+".src", f.Src, fontExt, "a font file (.woff2, .woff, .ttf or .otf)"); err != nil {
			return err
		}
		weight, err := Weight(f.Weight)
		if err != nil {
			return configError(file, key+".weight", "%v", err)
		}
		style, err := Style(f.Style)
		if err != nil {
			return configError(file, key+".style", "%v", err)
		}
		t.FontFiles = append(t.FontFiles, modules.FontFile{Family: family, Src: f.Src, Format: fontFormats[strings.ToLower(path.Ext(f.Src))], Weight: weight, Style: style})
		present(key+".src", f.Src)
	}
	return nil
}

// colours derives the palette, applies `tokens:` and checks every pair again.
func colours(file string, c Config, t *modules.Theme) ([]Finding, error) {
	main, err := colour(file, "main", c.Main)
	if err != nil {
		return nil, err
	}
	accent, err := colour(file, "accent", c.Accent)
	if err != nil {
		return nil, err
	}
	over, err := overrides(file, c.Tokens)
	if err != nil {
		return nil, err
	}
	p := Derive(main, accent)
	if p == nil && len(over) == 0 {
		return nil, nil
	}
	tokens := map[string]map[string]RGB{}
	for _, s := range Schemes {
		tokens[s] = map[string]RGB{}
		for k, v := range Shipped[s] {
			tokens[s][k] = MustHex(v)
		}
	}
	if p != nil {
		tokens = p.Tokens
		m, a := p.Main.Hex(), p.Accent.Hex()
		t.Main, t.Accent, t.Derived, t.Adjustments = &m, &a, append(t.Derived, p.Derived...), append(t.Adjustments, p.Adjustments...)
	}
	for s, o := range over {
		for k, v := range o {
			tokens[s][k] = v
		}
	}
	t.Light, t.Dark = themedHex(tokens["light"]), themedHex(tokens["dark"])
	t.Checks = Checks(tokens)
	var out []Finding
	for _, pr := range Verify(tokens) {
		out = append(out, Finding{Key: "tokens", Detail: pr.Scheme + ": " + pr.Detail})
	}
	return out, nil
}
