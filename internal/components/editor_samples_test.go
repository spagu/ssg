package components

// The component calls the editor-ui panel writes (GO-174) must be calls this
// parser reads back as the panel meant them. The samples live beside the
// panel's tests, which check that the panel writes exactly these calls; this
// test checks the other side: every call is found, named, and resolves to the
// typed props listed with it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type editorSamples struct {
	Components []ManifestComponent `json:"components"`
	Samples    []struct {
		Component string                 `json:"component"`
		Call      string                 `json:"call"`
		Props     map[string]interface{} `json:"props"`
	} `json:"samples"`
}

// componentFromManifest rebuilds a component's contract from its published form.
func componentFromManifest(m ManifestComponent) *Component {
	c := &Component{Name: m.Name, Props: map[string]Prop{}}
	for _, p := range m.Props {
		c.Props[p.Name] = Prop{Type: p.Type, Required: p.Required, Default: p.Default, Enum: p.Enum}
	}
	return c
}

// normalised compares values the way JSON writes them (3 and 3.0 alike).
func normalised(t *testing.T, v interface{}) interface{} {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEditorWrittenCallsParse(t *testing.T) {
	path := filepath.Join("..", "..", "workers", "editor-ui", "test", "widget-samples.json")
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed test file in this repository
	if err != nil {
		t.Fatal(err)
	}
	var file editorSamples
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	byName := map[string]*Component{}
	for _, m := range file.Components {
		byName[m.Name] = componentFromManifest(m)
	}
	if len(file.Samples) == 0 {
		t.Fatal("no samples")
	}
	for _, s := range file.Samples {
		c := byName[s.Component]
		if c == nil {
			t.Fatalf("%s: no component %q in the samples", s.Call, s.Component)
		}
		doc := "Before the call.\n\n" + s.Call + "\n\nAfter it.\n"
		calls := FindCalls(doc)
		if len(calls) != 1 || calls[0].Raw != s.Call || calls[0].Name != s.Component {
			t.Fatalf("%s: found %+v", s.Call, calls)
		}
		props, err := c.Resolve(calls[0])
		if err != nil {
			t.Fatalf("%s: %v", s.Call, err)
		}
		if got, want := normalised(t, props), normalised(t, s.Props); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got  %v\n want %v", s.Call, got, want)
		}
	}
}
