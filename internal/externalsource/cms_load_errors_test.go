package externalsource

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestCMSLoadFailures: an unknown adapter, a driver that will not open, and a
// database that does not answer each fail with the stage named.
func TestCMSLoadFailures(t *testing.T) {
	if _, err := (CMSConnector{}).Load(cmsSource("ghost", "x.db", "content")); err == nil || !strings.Contains(err.Error(), "config") {
		t.Errorf("unknown adapter: %v", err)
	}

	missing := cmsSource("wordpress", filepath.Join(t.TempDir(), "no", "such", "dir", "x.db"), "content")
	if _, err := (CMSConnector{}).Load(missing); err == nil || !strings.Contains(err.Error(), "connect") {
		t.Errorf("unreachable database: %v", err)
	}

	old := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errors.New("driver gone") }
	t.Cleanup(func() { sqlOpen = old })
	if _, err := (CMSConnector{}).Load(cmsSource("wordpress", "x.db", "content")); err == nil || !strings.Contains(err.Error(), "driver gone") {
		t.Errorf("driver: %v", err)
	}
}

func TestConnectorForDatabases(t *testing.T) {
	if _, ok := connectorFor(Source{Type: "sql"}, FileConnector{}, nil).(SQLConnector); !ok {
		t.Error("type sql must get the SQL connector")
	}
	if _, ok := connectorFor(Source{Type: "cms"}, FileConnector{}, nil).(CMSConnector); !ok {
		t.Error("type cms must get the CMS connector")
	}
}
