package dts

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// sourceMap maps the lines of a declaration file to the original source:
// lines[i] is where generated line i+1 came from.
type sourceMap struct {
	lines []origin
}

// origin is a position in an original source file; ok is false for a
// generated line with no mapping or a source outside the package root.
type origin struct {
	file string
	line int
	ok   bool
}

// rawSourceMap is the part of a version 3 source map the extractor reads.
type rawSourceMap struct {
	Version    int      `json:"version"`
	SourceRoot string   `json:"sourceRoot"`
	Sources    []string `json:"sources"`
	Mappings   string   `json:"mappings"`
}

// parseSourceMap reads a source map stored at mapPath (relative to the
// package root, "/" separated). Source paths become relative to the root.
func parseSourceMap(data []byte, mapPath string) (*sourceMap, error) {
	var raw rawSourceMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("source map: %w", err)
	}
	if raw.Version != 3 {
		return nil, fmt.Errorf("source map: version %d, only version 3 is read", raw.Version)
	}
	sources := make([]string, len(raw.Sources))
	for i, s := range raw.Sources {
		sources[i] = sourcePath(path.Dir(mapPath), raw.SourceRoot, s)
	}
	return decodeMappings(raw.Mappings, sources)
}

// sourcePath resolves a source entry against the map's directory and the
// source root; "" when it is a URL or leaves the package root.
func sourcePath(dir, root, src string) string {
	if strings.Contains(src, "://") || strings.Contains(root, "://") || path.IsAbs(src) {
		return ""
	}
	p := path.Clean(path.Join(dir, root, src))
	if p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		return ""
	}
	return p
}

// decodeMappings reads the "mappings" string. For each generated line it
// keeps the first segment that names a source position. Source index and
// line are deltas that run across lines; the generated column restarts on
// each line and is not needed.
func decodeMappings(mappings string, sources []string) (*sourceMap, error) {
	m := &sourceMap{}
	srcIdx, srcLine := 0, 0
	for _, line := range strings.Split(mappings, ";") {
		var at origin
		for _, seg := range strings.Split(line, ",") {
			if seg == "" {
				continue
			}
			fields, err := decodeVLQ(seg)
			if err != nil {
				return nil, err
			}
			if len(fields) < 4 {
				continue
			}
			srcIdx += fields[1]
			srcLine += fields[2]
			if !at.ok && srcIdx >= 0 && srcIdx < len(sources) && sources[srcIdx] != "" {
				at = origin{file: sources[srcIdx], line: srcLine + 1, ok: true}
			}
		}
		m.lines = append(m.lines, at)
	}
	return m, nil
}

// lookup returns the original file and line of a 1-based generated line.
func (m *sourceMap) lookup(line int) (string, int, bool) {
	if m == nil || line < 1 || line > len(m.lines) {
		return "", 0, false
	}
	o := m.lines[line-1]
	return o.file, o.line, o.ok
}

// base64Digits is the alphabet of source map VLQ digits.
const base64Digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// decodeVLQ decodes one segment: base64 digits of 5 value bits plus a
// continuation bit, each number's lowest bit its sign.
func decodeVLQ(seg string) ([]int, error) {
	var out []int
	value, shift := 0, 0
	for i := 0; i < len(seg); i++ {
		digit := strings.IndexByte(base64Digits, seg[i])
		if digit < 0 {
			return nil, fmt.Errorf("source map: bad character %q in mappings", seg[i])
		}
		if shift > 30 {
			return nil, fmt.Errorf("source map: number too large in mappings")
		}
		value |= (digit & 31) << shift
		if digit&32 != 0 {
			shift += 5
			continue
		}
		n := value >> 1
		if value&1 == 1 {
			n = -n
		}
		out = append(out, n)
		value, shift = 0, 0
	}
	if shift != 0 {
		return nil, fmt.Errorf("source map: truncated number in mappings")
	}
	return out, nil
}
