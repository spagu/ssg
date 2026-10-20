package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestBuildBreadcrumbOptions(t *testing.T) {
	off := false
	if o := buildBreadcrumbOptions(config.BreadcrumbsConfig{Enabled: &off}); !o.Off {
		t.Error("breadcrumbs: false must turn the list off")
	}
	o := buildBreadcrumbOptions(config.BreadcrumbsConfig{Home: "Start",
		Sections: map[string]config.BreadcrumbSection{"posts": {Name: "Wpisy", URL: "/blog/"}}})
	if o.Off || o.Home != "Start" || o.Sections["posts"].URL != "/blog/" {
		t.Errorf("options = %+v", o)
	}
	if o := buildBreadcrumbOptions(config.BreadcrumbsConfig{}); o.Off || o.Sections != nil {
		t.Errorf("unset must keep the derived list: %+v", o)
	}
}
