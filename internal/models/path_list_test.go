package models

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// TestPathListDecodes: one string or a list, in each config format (#386).
func TestPathListDecodes(t *testing.T) {
	type doc struct {
		Prefix PathList `yaml:"prefix" toml:"prefix" json:"prefix"`
	}
	one, two := PathList{"/openbsd/"}, PathList{"/openbsd/", "/netbsd/"}

	for name, c := range map[string]struct {
		decode func(string, *doc) error
		one    string
		two    string
	}{
		"yaml": {func(s string, d *doc) error { return yaml.Unmarshal([]byte(s), d) },
			"prefix: /openbsd/", "prefix: [/openbsd/, /netbsd/]"},
		"json": {func(s string, d *doc) error { return json.Unmarshal([]byte(s), d) },
			`{"prefix":"/openbsd/"}`, `{"prefix":["/openbsd/","/netbsd/"]}`},
		"toml": {func(s string, d *doc) error { _, err := toml.Decode(s, d); return err },
			`prefix = "/openbsd/"`, `prefix = ["/openbsd/", "/netbsd/"]`},
	} {
		var a, b doc
		if err := c.decode(c.one, &a); err != nil || !reflect.DeepEqual(a.Prefix, one) {
			t.Errorf("%s one: %v %v", name, a.Prefix, err)
		}
		if err := c.decode(c.two, &b); err != nil || !reflect.DeepEqual(b.Prefix, two) {
			t.Errorf("%s list: %v %v", name, b.Prefix, err)
		}
	}
}

func TestPathListRejectsOtherShapes(t *testing.T) {
	var l PathList
	if err := yaml.Unmarshal([]byte("{a: b}"), &l); err == nil {
		t.Error("yaml mapping accepted")
	}
	if err := json.Unmarshal([]byte(`{"a":1}`), &l); err == nil {
		t.Error("json object accepted")
	}
	if err := l.UnmarshalTOML([]interface{}{"/a/", int64(1)}); err == nil {
		t.Error("toml list with a number accepted")
	}
	if err := l.UnmarshalTOML(int64(1)); err == nil {
		t.Error("toml number accepted")
	}
}

// TestPathListMatchesWholeSegments: /net/ is not a prefix of /netbsd/x/.
func TestPathListMatchesWholeSegments(t *testing.T) {
	cases := []struct {
		list PathList
		path string
		want bool
	}{
		{PathList{"/net/"}, "/netbsd/x/", false},
		{PathList{"/net/"}, "/net/x/", true},
		{PathList{"net"}, "/net", true},
		{PathList{"/openbsd"}, "/openbsd/x.html", true},
		{PathList{"/a/b/"}, "/a/bc/", false},
		{PathList{"/"}, "/anything/", true},
		{PathList{"", "/x/"}, "/y/", false}, // a blank entry is not "/"
		{PathList{"/x/", "/y/"}, "/y/z/", true},
		{nil, "/y/", false},
		{PathList{"/x/"}, "", false},
	}
	for _, c := range cases {
		if got := c.list.MatchesPath(c.path); got != c.want {
			t.Errorf("%v.MatchesPath(%q) = %v, want %v", c.list, c.path, got, c.want)
		}
	}
	if (PathList{" ", ""}).Set() || !(PathList{"", "/x/"}).Set() || PathList(nil).Set() {
		t.Error("Set")
	}
}
