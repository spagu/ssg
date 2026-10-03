package php

import (
	"encoding/json"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// composerJSON is the part of composer.json the extractor reads.
type composerJSON struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Autoload struct {
		PSR4     map[string]pathList `json:"psr-4"`
		PSR0     map[string]pathList `json:"psr-0"`
		Classmap []string            `json:"classmap"`
		Files    []string            `json:"files"`
	} `json:"autoload"`
}

// pathList is an autoload target: one path or a list of them.
type pathList []string

// UnmarshalJSON accepts "src/" as well as ["src/", "lib/"].
func (p *pathList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*p = pathList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*p = many
	return nil
}

// readComposer reads composer.json from the root; a missing file is an
// empty one, a malformed one a diagnostic.
func readComposer(root *os.Root) (*composerJSON, []apisource.Diagnostic) {
	cj := &composerJSON{}
	data, err := root.ReadFile("composer.json")
	if err != nil {
		return cj, nil
	}
	if err := json.Unmarshal(data, cj); err != nil {
		return &composerJSON{}, []apisource.Diagnostic{{Severity: apisource.Error, File: "composer.json",
			Message: "cannot read composer.json: " + err.Error()}}
	}
	return cj, nil
}

// autoloadPaths returns the files and directories the autoload section
// names, cleaned, without repeats and sorted. An empty path ("" in PSR-4)
// is the root.
func (cj *composerJSON) autoloadPaths() []string {
	set := map[string]bool{}
	add := func(list []string) {
		for _, p := range list {
			set[path.Clean("./"+strings.ReplaceAll(p, "\\", "/"))] = true
		}
	}
	for _, m := range []map[string]pathList{cj.Autoload.PSR4, cj.Autoload.PSR0} {
		for _, list := range m {
			add(list)
		}
	}
	add(cj.Autoload.Classmap)
	add(cj.Autoload.Files)
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// packageName is the configured name, else composer.json's without its
// vendor ("acme/textkit" → "textkit"), else the root directory's name.
func packageName(cfgName, root string, cj *composerJSON) string {
	switch {
	case cfgName != "":
		return cfgName
	case cj.Name != "":
		return cj.Name[strings.LastIndex(cj.Name, "/")+1:]
	}
	return dirName(root)
}
