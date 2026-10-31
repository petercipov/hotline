package spec

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
)

// errXML marks a body that is not one well-formed XML document the proxy
// is willing to read.
var errXML = errors.New("not readable XML")

// What a schema says an XML element holds.
const (
	// xmlAny is a schema that names no type: the element is read as it
	// comes, as text, or as an object of its child elements.
	xmlAny = iota
	xmlText
	xmlObject
	xmlArray
)

// xmlShape is what the schema and its OpenAPI xml notes say about an XML
// element, so the proxy can read the element into the tree a JSON body
// would give.
type xmlShape struct {
	kind  int
	props []xmlProp
	// item is the shape of a list's items, and itemName their element's
	// name.
	item     *xmlShape
	itemName string
	// wrapped says the list has an outer element around its items.
	wrapped bool
}

// xmlProp is one property of an object: the key it has in the tree, the
// element or attribute it is read from, and its shape.
type xmlProp struct {
	key       string
	name      string
	attribute bool
	shape     *xmlShape
}

// xmlBody is how an XML body is read: the root element's name the schema
// expects (empty when it names none) and the root's shape (nil when the
// spec gives no schema, so the body is only read).
type xmlBody struct {
	root  string
	shape *xmlShape
}

func isXML(mediaType string) bool {
	return mediaType == "application/xml" || mediaType == "text/xml" || strings.HasSuffix(mediaType, "+xml")
}

// xmlBody builds how a body schema reads XML. The root element is the one
// the schema's xml name gives, or else the name of the shared schema its
// $ref points to, as OpenAPI tools write it.
func (b *builder) xmlBody(schema any, pointer string) *xmlBody {
	raw, _ := schema.(map[string]any)
	x := &xmlBody{shape: b.xmlShape(schema, pointer, map[string]*xmlShape{})}
	resolved, at := b.object(schema, pointer, b.name)
	component := ""
	if name, ok := strings.CutPrefix(at, "/components/schemas/"); ok && !strings.Contains(name, "/") {
		component = strings.NewReplacer("~1", "/", "~0", "~").Replace(name)
	}
	x.root = elementName(raw, resolved, component)
	return x
}

// xmlShape builds the shape of the schema at pointer. A schema met again,
// through a $ref that goes round, shares its shape.
func (b *builder) xmlShape(v any, pointer string, seen map[string]*xmlShape) *xmlShape {
	raw, _ := v.(map[string]any)
	m, at := b.object(v, pointer, b.name)
	if m == nil {
		return &xmlShape{}
	}
	if s, ok := seen[at]; ok {
		return s
	}
	s := &xmlShape{}
	seen[at] = s
	types := kinds(raw, m)
	props, hasProps := m["properties"].(map[string]any)
	items, hasItems := m["items"]
	switch {
	case slices.Contains(types, "array") || hasItems && len(types) == 0:
		s.kind = xmlArray
		s.wrapped, _ = notes(raw)["wrapped"].(bool)
		if !s.wrapped {
			s.wrapped, _ = notes(m)["wrapped"].(bool)
		}
		s.item = b.xmlShape(items, at+"/items", seen)
		itemRaw, _ := items.(map[string]any)
		itemResolved, _ := b.object(items, at+"/items", b.name)
		s.itemName = elementName(itemRaw, itemResolved, "")
	case slices.Contains(types, "object") || hasProps && len(types) == 0:
		s.kind = xmlObject
		for _, key := range slices.Sorted(maps.Keys(props)) {
			propPointer := at + "/properties" + token(key)
			propRaw, _ := props[key].(map[string]any)
			propResolved, _ := b.object(props[key], propPointer, b.name)
			attribute, _ := notes(propRaw)["attribute"].(bool)
			s.props = append(s.props, xmlProp{
				key:       key,
				name:      elementName(propRaw, propResolved, key),
				attribute: attribute,
				shape:     b.xmlShape(props[key], propPointer, seen),
			})
		}
	case len(types) > 0:
		s.kind = xmlText
	}
	return s
}

// notes are a schema's OpenAPI xml notes.
func notes(m map[string]any) map[string]any {
	n, _ := m["xml"].(map[string]any)
	return n
}

// elementName is the name the xml notes give, the property's own first,
// or else fallback.
func elementName(raw, resolved map[string]any, fallback string) string {
	for _, m := range []map[string]any{raw, resolved} {
		if name, ok := notes(m)["name"].(string); ok && name != "" {
			return name
		}
	}
	return fallback
}

// element is one element of a read XML body.
type element struct {
	name     string
	attrs    map[string]string
	text     strings.Builder
	children []*element
}

// read reads an XML body into the tree a JSON body would give, or says
// what is wrong with it: a body that isn't one well-formed document, and
// one whose root is not the element the schema names.
func (x *xmlBody) read(data []byte) (any, *Result) {
	root, err := parseXML(data)
	if err != nil {
		return nil, &Result{Part: partBody, Place: whole, Rule: "xml"}
	}
	if x.root != "" && root.name != x.root {
		return nil, &Result{Part: partBody, Place: whole, Rule: "xml_root", Expected: x.root}
	}
	if x.shape == nil {
		return nil, nil
	}
	return x.shape.value(root), nil
}

// parseXML reads one XML document. A DOCTYPE is refused, so no entity is
// ever expanded, and nothing outside the body is ever fetched.
func parseXML(data []byte) (*element, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *element
	var open []*element
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errXML
		}
		switch t := tok.(type) {
		case xml.StartElement:
			e := &element{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Space != "xmlns" && a.Name.Local != "xmlns" {
					e.attrs[a.Name.Local] = a.Value
				}
			}
			switch {
			case len(open) > 0:
				parent := open[len(open)-1]
				parent.children = append(parent.children, e)
			case root != nil:
				return nil, errXML
			default:
				root = e
			}
			open = append(open, e)
		case xml.EndElement:
			open = open[:len(open)-1]
		case xml.CharData:
			if len(open) > 0 {
				open[len(open)-1].text.Write(t)
			} else if len(bytes.TrimSpace(t)) > 0 {
				return nil, errXML
			}
		case xml.Directive:
			return nil, errXML
		}
	}
	if root == nil || len(open) > 0 {
		return nil, errXML
	}
	return root, nil
}

// value reads an element by its shape. Text stays text here; coerce later
// turns it into the number or true/false the schema wants.
func (s *xmlShape) value(e *element) any {
	switch s.kind {
	case xmlText:
		return e.text.String()
	case xmlObject:
		return s.object(e)
	case xmlArray:
		return s.list(e.children, s.itemName)
	}
	return loose(e)
}

func (s *xmlShape) object(e *element) map[string]any {
	out := map[string]any{}
	keys := map[string]bool{}
	for _, p := range s.props {
		keys[p.key] = true
		if !p.attribute {
			continue
		}
		if v, ok := e.attrs[p.name]; ok {
			out[p.key] = v
		}
	}
	for _, child := range e.children {
		p := s.prop(child.name)
		switch {
		case p == nil:
			// An element the schema doesn't name is kept, so a schema that
			// forbids extras reports it, unless it would take the key of a
			// property read from another name.
			if _, set := out[child.name]; !set && !keys[child.name] {
				out[child.name] = loose(child)
			}
		case p.shape.kind == xmlArray && !p.shape.wrapped:
			list, _ := out[p.key].([]any)
			out[p.key] = append(list, p.shape.item.value(child))
		default:
			if _, set := out[p.key]; !set {
				out[p.key] = p.shape.value(child)
			}
		}
	}
	return out
}

// prop is the property read from the element named name. An unwrapped
// list's items carry the item's name.
func (s *xmlShape) prop(name string) *xmlProp {
	for i, p := range s.props {
		if p.attribute {
			continue
		}
		want := p.name
		if p.shape.kind == xmlArray && !p.shape.wrapped {
			want = p.shape.itemName
			if want == "" {
				want = p.name
			}
		}
		if want == name {
			return &s.props[i]
		}
	}
	return nil
}

// list reads the items of a list among elements, by the items' name. With
// no name, every element is an item.
func (s *xmlShape) list(elements []*element, name string) []any {
	items := []any{}
	for _, e := range elements {
		if name == "" || e.name == name {
			items = append(items, s.item.value(e))
		}
	}
	return items
}

// loose reads an element the schema says nothing about: its text when it
// has no child elements, or else an object of them, with a name met more
// than once read as a list.
func loose(e *element) any {
	if len(e.children) == 0 {
		return e.text.String()
	}
	out := map[string]any{}
	for _, child := range e.children {
		v := loose(child)
		switch have := out[child.name].(type) {
		case nil:
			out[child.name] = v
		case []any:
			out[child.name] = append(have, v)
		default:
			out[child.name] = []any{have, v}
		}
	}
	return out
}
