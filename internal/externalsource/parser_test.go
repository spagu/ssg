package externalsource

import (
	"strings"
	"testing"
)

func parse(t *testing.T, format, input string, opts CSVOptions) interface{} {
	t.Helper()
	v, err := Parse(format, strings.NewReader(input), opts)
	if err != nil {
		t.Fatalf("Parse(%s): %v", format, err)
	}
	return v
}

func TestParseJSONYAMLTOML(t *testing.T) {
	j := parse(t, "json", `{"items":[{"name":"Go"}]}`, CSVOptions{}).(map[string]interface{})
	if j["items"].([]interface{})[0].(map[string]interface{})["name"] != "Go" {
		t.Fatalf("json = %#v", j)
	}
	y := parse(t, "yaml", "nav:\n  - title: Home\n    url: /\n", CSVOptions{}).(map[string]interface{})
	if y["nav"].([]interface{})[0].(map[string]interface{})["title"] != "Home" {
		t.Fatalf("yaml = %#v", y)
	}
	tm := parse(t, "toml", "[owner]\nname = \"spagu\"\n", CSVOptions{}).(map[string]interface{})
	if tm["owner"].(map[string]interface{})["name"] != "spagu" {
		t.Fatalf("toml = %#v", tm)
	}
}

func TestParseCSV(t *testing.T) {
	rows := parse(t, "csv", "name,price\nWidget,9.99\nGadget,19.99\n", CSVOptions{}).([]interface{})
	if len(rows) != 2 || rows[0].(map[string]interface{})["name"] != "Widget" ||
		rows[1].(map[string]interface{})["price"] != "19.99" {
		t.Fatalf("csv = %#v", rows)
	}
	// Header off → raw rows; custom delimiter.
	raw := parse(t, "csv", "a;b\nc;d\n", CSVOptions{Header: boolPtr(false), Delimiter: ";"}).([]interface{})
	if len(raw) != 2 || raw[0].([]string)[1] != "b" {
		t.Fatalf("raw csv = %#v", raw)
	}
	// Empty document with header.
	empty := parse(t, "csv", "", CSVOptions{}).([]interface{})
	if len(empty) != 0 {
		t.Fatalf("empty csv = %#v", empty)
	}
	if _, err := Parse("csv", strings.NewReader("a,b"), CSVOptions{Delimiter: "ab"}); err == nil {
		t.Fatal("multi-rune delimiter must error")
	}
	if _, err := Parse("csv", strings.NewReader("a,b\nc\"\n"), CSVOptions{}); err == nil {
		t.Fatal("malformed csv must error")
	}
}

func TestParseXML(t *testing.T) {
	doc := `<catalog version="2">
	  <product sku="W1"><name>Widget</name><price>9.99</price></product>
	  <product sku="G1"><name>Gadget</name></product>
	  <note>hello <b>world</b></note>
	</catalog>`
	v := parse(t, "xml", doc, CSVOptions{}).(map[string]interface{})
	catalog := v["catalog"].(map[string]interface{})
	if catalog["version"] != "2" {
		t.Fatalf("attr = %#v", catalog)
	}
	products := catalog["product"].([]interface{})
	first := products[0].(map[string]interface{})
	if len(products) != 2 || first["sku"] != "W1" || first["name"] != "Widget" {
		t.Fatalf("products = %#v", products)
	}
	note := catalog["note"].(map[string]interface{})
	if note["#text"] != "hello" || note["b"] != "world" {
		t.Fatalf("mixed content = %#v", note)
	}
	for _, bad := range []string{"", "<unclosed>"} {
		if _, err := Parse("xml", strings.NewReader(bad), CSVOptions{}); err == nil {
			t.Errorf("xml %q must error", bad)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for format, input := range map[string]string{
		"json": "{broken", "yaml": "a: [unclosed", "toml": "= broken", "cbor": "anything",
	} {
		if _, err := Parse(format, strings.NewReader(input), CSVOptions{}); err == nil {
			t.Errorf("%s: expected error", format)
		}
	}
}

func TestApplyTransformSelect(t *testing.T) {
	data := map[string]interface{}{"data": map[string]interface{}{"items": []interface{}{"a"}}}
	got, err := applyTransform(data, TransformConfig{Select: "data.items"})
	if err != nil || len(got.([]interface{})) != 1 {
		t.Fatalf("select = %#v, %v", got, err)
	}
	if same, err := applyTransform(data, TransformConfig{}); err != nil || same.(map[string]interface{})["data"] == nil {
		t.Fatal("empty select must be identity")
	}
	if _, err := applyTransform(data, TransformConfig{Select: "data.missing"}); err == nil {
		t.Fatal("missing key must error")
	}
	if _, err := applyTransform("scalar", TransformConfig{Select: "a"}); err == nil {
		t.Fatal("non-object must error")
	}
}

func TestRecordCount(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{[]interface{}{1, 2, 3}, 3},
		{[]map[string]string{{}}, 1},
		{map[string]interface{}{"a": 1, "b": 2}, 2},
		{nil, 0},
		{"scalar", 1},
	}
	for _, c := range cases {
		if got := recordCount(c.in); got != c.want {
			t.Errorf("recordCount(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
}
