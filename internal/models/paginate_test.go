package models

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPaginateSpecAcceptsBothForms: `paginate: 10` is the common case and the
// mapping form names the collection; both must parse.
func TestPaginateSpecAcceptsBothForms(t *testing.T) {
	var short struct {
		Paginate *PaginateSpec `yaml:"paginate"`
	}
	if err := yaml.Unmarshal([]byte("paginate: 10\n"), &short); err != nil {
		t.Fatalf("short form: %v", err)
	}
	if short.Paginate == nil || short.Paginate.Size != 10 || short.Paginate.Collection() != "posts" {
		t.Errorf("short form = %+v", short.Paginate)
	}

	var long struct {
		Paginate *PaginateSpec `yaml:"paginate"`
	}
	src := "paginate:\n  over: pages\n  size: 5\n  source: docs\n"
	if err := yaml.Unmarshal([]byte(src), &long); err != nil {
		t.Fatalf("long form: %v", err)
	}
	if long.Paginate.Over != "pages" || long.Paginate.Size != 5 || long.Paginate.Source != "docs" {
		t.Errorf("long form = %+v", long.Paginate)
	}
}

// TestPaginateSpecRejectsNonsense: a word or a list is neither a number nor a
// mapping, and the error names the problem rather than yielding a zero spec
// that silently paginates by the default.
func TestPaginateSpecRejectsNonsense(t *testing.T) {
	for _, src := range []string{"paginate: many\n", "paginate: [1, 2]\n"} {
		var v struct {
			Paginate *PaginateSpec `yaml:"paginate"`
		}
		if err := yaml.Unmarshal([]byte(src), &v); err == nil {
			t.Errorf("%q parsed as %+v", src, v.Paginate)
		}
	}
}
