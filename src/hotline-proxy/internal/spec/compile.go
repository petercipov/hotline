package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// specURL is the address the spec is known by inside the JSON Schema
// compiler. Nothing is ever fetched from it.
const specURL = "https://spec.hotline.invalid/openapi.json"

// errOutside marks a $ref to a document other than the spec.
var errOutside = errors.New("outside the spec")

// errBits marks a whole number too large for its format.
var errBits = errors.New("does not fit")

// builder turns the spec, read as plain data, into the checks of each
// route. Its schemas are compiled once, and $ref is followed only within
// the spec.
type builder struct {
	name     string
	doc      any
	compiler *jsonschema.Compiler
	// inline compiles schemas that need nothing else from the spec, such
	// as { type: string } on a header, apart from the spec, and inlined
	// keeps each one by its text, so the same schema is compiled once.
	inline  *jsonschema.Compiler
	inlined map[string]*jsonschema.Schema
	// inlineNext numbers the schemas given to inline.
	inlineNext int
	schemes    map[string]*credential
	problems   []error
	// reported holds the x-hotline-record marks already found broken.
	reported map[string]bool
}

// outside refuses every document but the spec itself, so a $ref to
// another file or address is a problem.
type outside struct{}

func (outside) Load(address string) (any, error) {
	return nil, fmt.Errorf("%w: %s", errOutside, address)
}

func newBuilder(name string, data []byte) *builder {
	b := &builder{name: name, schemes: map[string]*credential{}, reported: map[string]bool{}}
	var raw any
	_ = yaml.Unmarshal(data, &raw)
	plain, err := json.Marshal(normalize(raw))
	if err == nil {
		b.doc, err = jsonschema.UnmarshalJSON(bytes.NewReader(plain))
	}
	b.compiler = newCompiler()
	b.inline = newCompiler()
	b.inlined = map[string]*jsonschema.Schema{}
	if err == nil {
		err = b.compiler.AddResource(specURL, b.doc)
	}
	if err != nil {
		b.problem("%s cannot be read as JSON Schema: %v", name, err)
		return b
	}
	b.readSchemes()
	b.compileComponents()
	return b
}

// newCompiler makes a JSON Schema compiler set up the way the proxy
// checks: draft 2020-12, formats and JSON text checked, nothing fetched.
func newCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(outside{})
	c.AssertFormat()
	// A string holding JSON, marked with contentMediaType and
	// contentSchema, is read and checked too (Peter, 2026-10-30).
	c.RegisterVocabulary(jsonText())
	c.AssertVocabs()
	for _, f := range ignoredFormats() {
		c.RegisterFormat(&jsonschema.Format{Name: f, Validate: func(any) error { return nil }})
	}
	c.RegisterFormat(&jsonschema.Format{Name: "int32", Validate: bits(32)})
	c.RegisterFormat(&jsonschema.Format{Name: "int64", Validate: bits(64)})
	return c
}

// ignoredFormats are the formats the JSON Schema library knows but the
// proxy does not check. Only date-time, date, email, uuid, uri, ipv4, ipv6,
// int32 and int64 are checked as rules (Peter, 2026-10-29).
func ignoredFormats() []string {
	return []string{
		"json-pointer", "relative-json-pointer", "duration", "period", "hostname", "time",
		"iri", "uri-reference", "iri-reference", "uri-template", "semver", "regex",
		"idn-email", "idn-hostname",
	}
}

// bits checks that a whole number fits in a signed integer of that size.
func bits(size uint) func(any) error {
	limit := new(big.Int).Lsh(big.NewInt(1), size-1)
	low := new(big.Int).Neg(limit)
	return func(v any) error {
		n, ok := new(big.Int).SetString(fmt.Sprint(v), 10)
		if !ok {
			return nil
		}
		if n.Cmp(low) < 0 || n.Cmp(limit) >= 0 {
			return fmt.Errorf("%w in %d bits", errBits, size)
		}
		return nil
	}
}

func jsonNumber(text string) any { return json.Number(text) }

// normalize makes YAML data plain JSON data: every object key a string.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			x[key] = normalize(child)
		}
		return x
	case map[any]any:
		m := make(map[string]any, len(x))
		for key, child := range x {
			m[fmt.Sprint(key)] = normalize(child)
		}
		return m
	case []any:
		for i, child := range x {
			x[i] = normalize(child)
		}
	}
	return v
}

func (b *builder) problem(format string, args ...any) {
	b.problems = append(b.problems, fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...)))
}

// token is one step of a JSON pointer, with / and ~ escaped.
func token(name string) string {
	return "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}

// at finds the value at a JSON pointer, such as /components/schemas/Order.
func (b *builder) at(pointer string) (any, bool) {
	v := b.doc
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		var found bool
		switch x := v.(type) {
		case map[string]any:
			v, found = x[part]
		case []any:
			i, err := strconv.Atoi(part)
			found = err == nil && i >= 0 && i < len(x)
			if found {
				v = x[i]
			}
		}
		if !found {
			return nil, false
		}
	}
	return v, true
}

// object follows any $ref and returns the object found, with its pointer.
// A $ref to another file, or to nothing, is a problem.
func (b *builder) object(v any, pointer, where string) (map[string]any, string) {
	for range 16 {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, ""
		}
		ref, isRef := m["$ref"].(string)
		if !isRef {
			return m, pointer
		}
		if !strings.HasPrefix(ref, "#") {
			b.problem("%s: the $ref %s points to another file", where, ref)
			return nil, ""
		}
		pointer, _ = url.PathUnescape(strings.TrimPrefix(ref, "#"))
		if v, ok = b.at(pointer); !ok {
			b.problem("%s: the $ref %s points nowhere", where, ref)
			return nil, ""
		}
	}
	b.problem("%s: a $ref goes round in a circle", where)
	return nil, ""
}

// schema compiles the schema under pointer, nil when there is none.
func (b *builder) schema(owner map[string]any, pointer, where, what string) *jsonschema.Schema {
	v, ok := owner["schema"]
	if !ok {
		return nil
	}
	s, err := b.compileSchema(v, pointer+"/schema")
	if err != nil {
		b.problem("%s: the schema of %s cannot be used: %v", where, what, err)
	}
	return s
}

// compileSchema compiles the schema v found at pointer, the cheap way
// when it can. The compiler copies all it has read each time it meets a
// new place in the spec, so a check per call made loading grow with the
// calls times the shapes; the SSM spec took about 950 MB of short-lived
// memory (Peter, 2026-11-01). So a schema that is only a $ref within the
// spec uses the check of the shape it names, built once, and a schema
// that needs nothing else from the spec is compiled apart from it, once
// for each text.
func (b *builder) compileSchema(v any, pointer string) (*jsonschema.Schema, error) {
	if target, ok := onlyRef(v); ok {
		return b.compile(target)
	}
	if !standsAlone(v) {
		return b.compile(pointer)
	}
	// The spec was read as JSON, so it always writes back as JSON.
	text, _ := json.Marshal(v)
	if s, ok := b.inlined[string(text)]; ok {
		return s, nil
	}
	b.inlineNext++
	address := fmt.Sprintf("%s/inline/%d.json", strings.TrimSuffix(specURL, "/openapi.json"), b.inlineNext)
	var s *jsonschema.Schema
	err := b.inline.AddResource(address, v)
	if err == nil {
		s, err = b.inline.Compile(address)
	}
	if err != nil {
		// Built again in place, so the problem names where it is in the
		// spec. Only a broken spec gets here.
		return b.compile(pointer)
	}
	b.inlined[string(text)] = s
	return s, nil
}

// onlyRef is the pointer of the shape v names, when v holds nothing but a
// $ref within the spec.
func onlyRef(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	ref, ok := m["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#") {
		return "", false
	}
	target, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	return target, err == nil
}

// standsAlone tells whether a schema needs nothing else from the spec: it
// has no reference to elsewhere and names no place others could point to.
func standsAlone(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			switch key {
			case "$ref", "$dynamicRef", "$id", "$anchor", "$dynamicAnchor", "$schema", "$vocabulary":
				return false
			}
			if !standsAlone(value) {
				return false
			}
		}
	case []any:
		for _, value := range x {
			if !standsAlone(value) {
				return false
			}
		}
	}
	return true
}

func (b *builder) compile(pointer string) (*jsonschema.Schema, error) {
	return b.compiler.Compile(specURL + "#" + (&url.URL{Fragment: pointer}).EscapedFragment())
}

// compileComponents compiles every shared schema, so a broken one is found
// even when no route uses it.
func (b *builder) compileComponents() {
	schemas, _ := b.at("/components/schemas")
	m, _ := schemas.(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(m)) {
		if _, err := b.compile("/components/schemas" + token(name)); err != nil {
			b.problem("%s: the schema %s cannot be used: %v", b.name, name, err)
		}
	}
}

// readSchemes reads how apps log in. Every way but an API key is sent in
// the Authorization header; mutual TLS sends nothing the proxy can see.
func (b *builder) readSchemes() {
	schemes, _ := b.at("/components/securitySchemes")
	m, _ := schemes.(map[string]any)
	for name, v := range m {
		scheme, _ := b.object(v, "/components/securitySchemes"+token(name), b.name+" security scheme "+name)
		switch str(scheme, "type") {
		case "apiKey":
			b.schemes[name] = &credential{in: str(scheme, "in"), name: str(scheme, "name")}
		case "http", "oauth2", "openIdConnect":
			b.schemes[name] = &credential{in: "header", name: "Authorization"}
		}
	}
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// checks builds the checks of one route. template is the route as the
// spec writes it, method in lower case.
func (b *builder) checks(template, method, where string) *checks {
	itemPointer := "/paths" + token(template)
	itemValue, _ := b.at(itemPointer)
	item, itemPointer := b.object(itemValue, itemPointer, where)
	opPointer := itemPointer + token(method)
	op, _ := item[method].(map[string]any)
	c := &checks{
		params:   b.params(where, item["parameters"], itemPointer+"/parameters", op["parameters"], opPointer+"/parameters"),
		security: b.security(op),
	}
	if v, ok := op["requestBody"]; ok {
		body, pointer := b.object(v, opPointer+"/requestBody", where)
		c.body = b.bodySpec(body, pointer, where, "the request body")
		if c.body != nil {
			c.body.required, _ = body["required"].(bool)
		}
	}
	if responses, ok := op["responses"].(map[string]any); ok {
		c.replies = map[string]*reply{}
		for code, v := range responses {
			r, pointer := b.object(v, opPointer+"/responses"+token(code), where)
			if len(code) == 3 && strings.HasSuffix(strings.ToUpper(code), "XX") {
				code = strings.ToUpper(code)
			}
			c.replies[code] = b.reply(r, pointer, where, code)
		}
	}
	b.clashes(c, where)
	return c
}

// params are the route's parameters: those of the path first, then the
// method's own, which win over a path one of the same name.
func (b *builder) params(where string, itemParams any, itemPointer string, opParams any, opPointer string) []param {
	var params []param
	for _, set := range []struct {
		list    any
		pointer string
	}{{itemParams, itemPointer}, {opParams, opPointer}} {
		list, _ := set.list.([]any)
		for i, v := range list {
			m, pointer := b.object(v, fmt.Sprintf("%s/%d", set.pointer, i), where)
			p := b.param(m, pointer, where)
			if p == nil {
				continue
			}
			same := slices.IndexFunc(params, func(q param) bool {
				return q.in == p.in && strings.EqualFold(q.name, p.name)
			})
			if same >= 0 {
				params[same] = *p
				continue
			}
			params = append(params, *p)
		}
	}
	return params
}

func (b *builder) param(m map[string]any, pointer, where string) *param {
	p := &param{name: str(m, "name"), in: str(m, "in"), explode: true}
	if p.name == "" || p.in == "" {
		return nil
	}
	if mark, marked := m["x-hotline-record"]; marked {
		p.record = b.headerMark(mark, p.in, p.name, where)
	}
	// OpenAPI says these headers are described elsewhere, so a header
	// parameter with their name is ignored.
	if p.in == partHeader && slices.Contains([]string{"accept", "content-type", "authorization"}, strings.ToLower(p.name)) {
		return nil
	}
	p.required, _ = m["required"].(bool)
	style := str(m, "style")
	p.deepObject = style == "deepObject"
	if explode, ok := m["explode"].(bool); ok {
		p.explode = explode
	} else if style != "" && style != "form" {
		p.explode = false
	}
	p.schema = b.schema(m, pointer, where, p.in+" "+p.name)
	return p
}

// security is the method's own list of ways to log in, or the spec's.
func (b *builder) security(op map[string]any) [][]credential {
	v, ok := op["security"]
	if !ok {
		v, _ = b.at("/security")
	}
	list, _ := v.([]any)
	var ways [][]credential
	for _, item := range list {
		requirement, _ := item.(map[string]any)
		var way []credential
		for _, name := range slices.Sorted(maps.Keys(requirement)) {
			if cred := b.schemes[name]; cred != nil {
				way = append(way, *cred)
			}
		}
		if len(way) == 0 {
			// A way that needs nothing: the route is open to all.
			return nil
		}
		ways = append(ways, way)
	}
	return ways
}

func (b *builder) bodySpec(m map[string]any, pointer, where, what string) *bodySpec {
	listed, ok := m["content"].(map[string]any)
	if !ok {
		return nil
	}
	body := &bodySpec{}
	for _, mediaType := range slices.Sorted(maps.Keys(listed)) {
		media, _ := listed[mediaType].(map[string]any)
		mark, marked := media["x-hotline-body"]
		if marked && mark != "json" && mark != "xml" {
			b.problem("%s: x-hotline-body of %s %s must be json or xml", where, what, mediaType)
		}
		at := pointer + "/content" + token(mediaType)
		ct := content{
			mediaType: mediaType,
			json:      mark == "json",
			xmlMark:   mark == "xml",
			schema:    b.schema(media, at, where, what+" "+mediaType),
		}
		schema, hasSchema := media["schema"]
		if hasSchema {
			ct.fields = b.fields(schema, at+"/schema")
		}
		// XML is read by the schema's shape, built only where a body can
		// be XML: by the mark, by its type, or by a wildcard type.
		if ct.xmlMark || isXML(strings.ToLower(mediaType)) || strings.Contains(mediaType, "*") {
			ct.xml = &xmlBody{}
			if hasSchema {
				ct.xml = b.xmlBody(schema, at+"/schema")
			}
		}
		body.content = append(body.content, ct)
	}
	return body
}

func (b *builder) reply(m map[string]any, pointer, where, code string) *reply {
	r := &reply{body: b.bodySpec(m, pointer, where, "the "+code+" reply")}
	headers, _ := m["headers"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		if strings.EqualFold(name, "Content-Type") {
			continue
		}
		h, hp := b.object(headers[name], pointer+"/headers"+token(name), where)
		if _, marked := h["x-hotline-record"]; marked {
			b.problem("%s: x-hotline-record on the %s reply header %s: a mark goes only on a header of the call", where, code, name)
		}
		required, _ := h["required"].(bool)
		r.headers = append(r.headers, param{
			name: name, in: partHeader, required: required,
			schema: b.schema(h, hp, where, "the "+code+" reply header "+name),
		})
	}
	return r
}
