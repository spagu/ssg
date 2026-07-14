package externalsource

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestErrorUnwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := fail(Source{Name: "s", Type: "file"}, "read", cause)
	if !errors.Is(err, cause) {
		t.Fatal("Unwrap must expose the cause")
	}
}

func TestParseYAMLReadError(t *testing.T) {
	if _, err := parseYAML(failingReader{}); err == nil {
		t.Fatal("read error must surface")
	}
}

func TestNormalizeValueInterfaceKeys(t *testing.T) {
	in := map[interface{}]interface{}{1: "a", "b": []interface{}{map[interface{}]interface{}{true: "c"}}}
	out := normalizeValue(in).(map[string]interface{})
	if out["1"] != "a" {
		t.Fatalf("normalized = %#v", out)
	}
	nested := out["b"].([]interface{})[0].(map[string]interface{})
	if nested["true"] != "c" {
		t.Fatalf("nested = %#v", nested)
	}
}

func TestParseXMLRepeatedThriceAndBadToken(t *testing.T) {
	v := parse(t, "xml", "<r><i>1</i><i>2</i><i>3</i></r>", CSVOptions{}).(map[string]interface{})
	items := v["r"].(map[string]interface{})["i"].([]interface{})
	if len(items) != 3 || items[2] != "3" {
		t.Fatalf("items = %#v", items)
	}
	if _, err := Parse("xml", strings.NewReader("<r>&undefined;</r>"), CSVOptions{}); err == nil {
		t.Fatal("undefined entity must error")
	}
	if _, err := Parse("xml", strings.NewReader("&broken"), CSVOptions{}); err == nil {
		t.Fatal("bad prolog token must error")
	}
}
