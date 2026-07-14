package externalsource

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Parse decodes raw source bytes in the given format. Parsers are independent
// of the transport, so the same code will serve file and HTTP sources.
func Parse(format string, r io.Reader, opts CSVOptions) (interface{}, error) {
	switch format {
	case "json":
		return parseJSON(r)
	case "yaml":
		return parseYAML(r)
	case "toml":
		return parseTOML(r)
	case "csv":
		return parseCSV(r, opts)
	case "xml":
		return parseXML(r)
	}
	return nil, fmt.Errorf("no parser for format %q", format)
}

// parseJSON decodes any JSON document.
func parseJSON(r io.Reader) (interface{}, error) {
	var v interface{}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return v, nil
}

// parseYAML decodes any YAML document, normalizing interface-keyed maps so
// html/template can index the result.
func parseYAML(r io.Reader) (interface{}, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var v interface{}
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	return normalizeValue(v), nil
}

// parseTOML decodes a TOML document into a string-keyed map.
func parseTOML(r io.Reader) (interface{}, error) {
	var v map[string]interface{}
	if _, err := toml.NewDecoder(r).Decode(&v); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}
	return normalizeValue(v), nil
}

// parseCSV decodes CSV rows. With a header (default) each row becomes a
// map[column]value; without one each row stays a plain []string.
func parseCSV(r io.Reader, opts CSVOptions) (interface{}, error) {
	cr := csv.NewReader(r)
	if d := opts.Delimiter; d != "" {
		runes := []rune(d)
		if len(runes) != 1 {
			return nil, fmt.Errorf("csv delimiter must be one character, got %q", d)
		}
		cr.Comma = runes[0]
	}
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing CSV: %w", err)
	}
	header := opts.Header == nil || *opts.Header
	if !header {
		out := make([]interface{}, len(rows))
		for i, row := range rows {
			out[i] = row
		}
		return out, nil
	}
	if len(rows) == 0 {
		return []interface{}{}, nil
	}
	cols := rows[0]
	out := make([]interface{}, 0, len(rows)-1)
	for _, row := range rows[1:] {
		record := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if i < len(row) {
				record[strings.TrimSpace(col)] = row[i]
			}
		}
		out = append(out, record)
	}
	return out, nil
}

// normalizeValue converts map[interface{}]interface{} (YAML) into
// map[string]interface{} recursively so templates can index it.
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, vv := range val {
			out[fmt.Sprintf("%v", k)] = normalizeValue(vv)
		}
		return out
	case map[string]interface{}:
		for k, vv := range val {
			val[k] = normalizeValue(vv)
		}
		return val
	case []interface{}:
		for i, vv := range val {
			val[i] = normalizeValue(vv)
		}
		return val
	}
	return v
}
