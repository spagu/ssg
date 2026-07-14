package externalsource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// FileConnector loads local file sources. Paths come from trusted local
// configuration (same trust model as data_dir), so any project-relative or
// absolute path is allowed; a size cap guards against runaway files.
type FileConnector struct{}

// Load reads, size-checks, parses and transforms one file source.
func (FileConnector) Load(src Source) (*Result, error) {
	info, err := os.Stat(src.Path)
	if err != nil {
		return nil, fail(src, "read", err)
	}
	if info.Size() > src.MaxSize {
		return nil, fail(src, "read",
			fmt.Errorf("file is %d bytes, exceeding the %d-byte limit (defaults.max_response_size)", info.Size(), src.MaxSize))
	}
	raw, err := os.ReadFile(src.Path) // #nosec G304 -- path from trusted local config, like data_dir
	if err != nil {
		return nil, fail(src, "read", err)
	}
	data, err := Parse(src.Format, bytes.NewReader(raw), src.CSV)
	if err != nil {
		return nil, fail(src, "parse", err)
	}
	data, err = applyTransform(data, src.Transform)
	if err != nil {
		return nil, fail(src, "transform", err)
	}
	sum := sha256.Sum256(raw)
	return &Result{
		Name: src.Name,
		Type: src.Type,
		Data: data,
		Metadata: Metadata{
			SourceType:  src.Type,
			Identifier:  src.Path,
			FetchedAt:   time.Now(),
			Checksum:    hex.EncodeToString(sum[:]),
			RecordCount: recordCount(data),
			ContentType: src.Format,
		},
	}, nil
}
