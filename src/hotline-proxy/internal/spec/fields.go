package spec

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Field is one body field the spec marks x-hotline-record, copied from a
// call or a reply: the record attribute's name, and the value as a string,
// bool, int64 or float64.
type Field struct {
	Name  string
	Value any
}

// field is where a marked field is in a body, and what it is copied as.
type field struct {
	path  []string
	name  string
	types []string
}

// longestText is how many characters of a copied text a record holds.
const longestText = 256

// recordName is what a copied field's name may hold.
var recordName = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// ownNames start the names of the attributes the proxy writes itself, so a
// copied field can never pass for one of them.
var ownNames = []string{"hotline.", "http.", "url.", "server.", "network.", "error.", "dns.", "tls.", "log."}

// fields finds the marked fields of a body schema. Only properties are
// followed, through objects; a field inside a list has no one value, so
// it is never copied, but its mark is still checked.
func (b *builder) fields(v any, pointer string) []field {
	var found []field
	b.walk(v, pointer, nil, false, map[string]bool{}, &found)
	return found
}

func (b *builder) walk(v any, pointer string, path []string, inList bool, seen map[string]bool, found *[]field) {
	m, at := b.object(v, pointer, b.name)
	if m == nil || seen[at] {
		return
	}
	seen[at] = true
	defer delete(seen, at)
	props, _ := m["properties"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(props)) {
		raw, _ := props[name].(map[string]any)
		propPointer := at + "/properties" + token(name)
		resolved, resolvedAt := b.object(raw, propPointer, b.name)
		inner := append(slices.Clone(path), name)
		if mark, marked := raw["x-hotline-record"]; marked {
			f, ok := b.mark(mark, kinds(raw, resolved), propPointer)
			if ok && !inList {
				f.path = inner
				*found = append(*found, f)
			}
		}
		b.walk(raw, propPointer, inner, inList, seen, found)
		if items, ok := resolved["items"]; ok {
			b.walk(items, resolvedAt+"/items", inner, true, seen, found)
		}
	}
}

// mark reads one x-hotline-record mark. A problem with it is reported once,
// however many bodies share the schema.
func (b *builder) mark(mark any, types []string, pointer string) (field, bool) {
	name, isText := mark.(string)
	var problem string
	switch {
	case !isText:
		problem = "must be a name, such as messaging.destination.name"
	case !recordName.MatchString(name):
		problem = fmt.Sprintf("names %q, but a name may only hold letters, digits, dots and underscores", name)
	case slices.ContainsFunc(ownNames, func(own string) bool { return strings.HasPrefix(name, own) }):
		problem = fmt.Sprintf("names %s, which starts like the proxy's own names (%s)", name, strings.Join(ownNames, " "))
	case len(types) == 0 || slices.ContainsFunc(types, func(t string) bool { return !slices.Contains([]string{"string", "number", "integer", "boolean"}, t) }):
		problem = "must be on a field of text, a number, or true or false, with its type given"
	}
	if problem == "" {
		return field{name: name, types: types}, true
	}
	if !b.reported[pointer] {
		b.reported[pointer] = true
		b.problem("%s: x-hotline-record at %s %s", b.name, pointer, problem)
	}
	return field{}, false
}

// kinds are the types a property allows, null left out: its own, or those
// of the schema its $ref points to.
func kinds(raw, resolved map[string]any) []string {
	t, ok := raw["type"]
	if !ok {
		t = resolved["type"]
	}
	var list []string
	switch x := t.(type) {
	case string:
		list = []string{x}
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok {
				list = append(list, s)
			}
		}
	}
	return slices.DeleteFunc(list, func(s string) bool { return s == "null" })
}

// clashes reports names marked in both the call's body and a reply's on one
// route, since a record holds each name once.
func (b *builder) clashes(c *checks, where string) {
	if c.body == nil {
		return
	}
	asked := map[string]bool{}
	for _, ct := range c.body.content {
		for _, f := range ct.fields {
			asked[f.name] = true
		}
	}
	clashing := map[string]bool{}
	for _, rp := range c.replies {
		if rp == nil || rp.body == nil {
			continue
		}
		for _, ct := range rp.body.content {
			for _, f := range ct.fields {
				if asked[f.name] {
					clashing[f.name] = true
				}
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(clashing)) {
		b.problem("%s: x-hotline-record names %s in both the call and the reply", where, name)
	}
}

// copied are the marked fields found in a read body, each with the value
// it has, if that value is of the marked type.
func (c *content) copied(instance any) []Field {
	var out []Field
	for _, f := range c.fields {
		v := instance
		for _, step := range f.path {
			m, _ := v.(map[string]any)
			v = m[step]
		}
		if value, ok := plain(v, f.types); ok {
			out = append(out, Field{Name: f.name, Value: value})
		}
	}
	return out
}

func plain(v any, types []string) (any, bool) {
	switch x := v.(type) {
	case string:
		if !slices.Contains(types, "string") {
			return nil, false
		}
		if runes := []rune(x); len(runes) > longestText {
			x = string(runes[:longestText])
		}
		return x, true
	case bool:
		return x, slices.Contains(types, "boolean")
	case json.Number:
		if !slices.Contains(types, "integer") && !slices.Contains(types, "number") {
			return nil, false
		}
		if n, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			return n, true
		}
		f, err := strconv.ParseFloat(string(x), 64)
		return f, err == nil && !math.IsInf(f, 0)
	}
	return nil, false
}
