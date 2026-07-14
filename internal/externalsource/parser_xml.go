package externalsource

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// parseXML converts an XML document into template-friendly nested maps:
// attributes become plain keys, child elements become keys (repeated names
// collect into lists), and elements holding only text collapse to strings.
// Mixed content keeps its text under "#text".
func parseXML(r io.Reader) (interface{}, error) {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("parsing XML: no root element")
		}
		if err != nil {
			return nil, fmt.Errorf("parsing XML: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			node, err := xmlElement(dec, start)
			if err != nil {
				return nil, fmt.Errorf("parsing XML: %w", err)
			}
			return map[string]interface{}{start.Name.Local: node}, nil
		}
	}
}

// xmlElement reads one element (start token already consumed) to its end tag.
func xmlElement(dec *xml.Decoder, start xml.StartElement) (interface{}, error) {
	node := map[string]interface{}{}
	for _, attr := range start.Attr {
		node[attr.Name.Local] = attr.Value
	}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := xmlElement(dec, t)
			if err != nil {
				return nil, err
			}
			appendXMLChild(node, t.Name.Local, child)
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return finishXMLNode(node, strings.TrimSpace(text.String())), nil
		}
	}
}

// appendXMLChild inserts a child value, turning repeated names into lists.
func appendXMLChild(node map[string]interface{}, name string, child interface{}) {
	switch existing := node[name].(type) {
	case nil:
		node[name] = child
	case []interface{}:
		node[name] = append(existing, child)
	default:
		node[name] = []interface{}{existing, child}
	}
}

// finishXMLNode collapses text-only elements to strings and attaches mixed text.
func finishXMLNode(node map[string]interface{}, text string) interface{} {
	if len(node) == 0 {
		return text
	}
	if text != "" {
		node["#text"] = text
	}
	return node
}
