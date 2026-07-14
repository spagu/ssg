package externalsource

import "time"

// Metadata describes how a source's data was obtained. It is exposed to
// templates as .ExternalDataMeta.<name>.
type Metadata struct {
	SourceType  string    // "file" (later: http, sql, cms)
	Identifier  string    // safe identifier: file path, URL host+path, DSN-less DB name
	FetchedAt   time.Time // when the data was read this build
	FromCache   bool      // phase 2+: served from the disk cache
	Stale       bool      // phase 2+: stale-if-error result
	Checksum    string    // sha256 of the raw bytes
	RecordCount int       // top-level records after transform
	ContentType string    // parser format (yaml/json/toml/csv/xml)
}

// Result is one loaded source: its parsed data plus metadata.
type Result struct {
	Name     string
	Type     string
	Data     interface{}
	Metadata Metadata
}

// recordCount reports the number of top-level records in parsed data.
func recordCount(v interface{}) int {
	switch t := v.(type) {
	case []interface{}:
		return len(t)
	case []map[string]string:
		return len(t)
	case map[string]interface{}:
		return len(t)
	case nil:
		return 0
	}
	return 1
}
