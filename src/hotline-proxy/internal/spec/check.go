package spec

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Result is one thing a check found: the part of the call, the place in it,
// the rule it broke and what the spec expects. It never holds a value from
// the call.
type Result struct {
	Part     string
	Place    string
	Rule     string
	Expected string
}

// Body is the copy of a body the proxy kept for the checks.
type Body struct {
	Data []byte
	// TooLarge says the body was longer than the copy.
	TooLarge bool
}

// Exchange is a finished call, as the checks see it.
type Exchange struct {
	Method      string
	EscapedPath string
	RawQuery    string
	Header      http.Header
	Body        Body
	// Status is 0 when no reply came.
	Status      int
	ReplyHeader http.Header
	ReplyBody   Body
}

// Checked is what the checks found on each side, and why some checks were
// left out, if any were.
type Checked struct {
	Request []Result
	Reply   []Result
	Skipped string
}

// checks are what the spec says about one route: its parameters, who may
// call it, its body and its replies.
type checks struct {
	params []param
	// security lists the ways to log in, any one of which is enough. Nil
	// or a way with no credentials means none is needed.
	security [][]credential
	body     *bodySpec
	// replies are by status: "200", "2XX" or "default". Nil when the spec
	// lists none, so replies are not checked.
	replies map[string]*reply
}

type param struct {
	name     string
	in       string
	required bool
	// deepObject reads an object from name[key]=value query pairs.
	deepObject bool
	explode    bool
	schema     *jsonschema.Schema
}

type credential struct {
	in   string
	name string
}

type bodySpec struct {
	required bool
	content  []content
}

type content struct {
	mediaType string
	// schema is nil when the spec gives the type but no schema.
	schema *jsonschema.Schema
}

type reply struct {
	headers []param
	body    *bodySpec
}

const (
	partBody   = "body"
	partHeader = "header"
	partStatus = "status"
	whole      = "$"
)

// Check checks a finished call against the spec. A call with no route is
// not checked.
func (s *Spec) Check(e Exchange) Checked {
	r := s.match(e.Method, e.EscapedPath, e.Header)
	if r.segments == nil {
		return Checked{Skipped: "no_spec_route"}
	}
	var c Checked
	query, _ := url.ParseQuery(e.RawQuery)
	cookies := map[string]string{}
	for _, cookie := range (&http.Request{Header: e.Header}).Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	segments := strings.Split(strings.TrimSuffix(e.EscapedPath, "/"), "/")
	for _, p := range r.checks.params {
		values := p.values(r, segments, query, e.Header, cookies)
		c.Request = append(c.Request, p.check(values)...)
	}
	c.Request = append(c.Request, s.unknownQuery(r.checks.params, query)...)
	c.Request = append(c.Request, r.checks.loggedIn(e.Header, query, cookies)...)
	results, skipped := r.checks.body.check(e.Header.Get("Content-Type"), e.Body)
	c.Request = append(c.Request, results...)
	c.Skipped = skipped
	if e.Status == 0 || r.checks.replies == nil {
		return c
	}
	rp := r.checks.reply(e.Status)
	if rp == nil {
		c.Reply = []Result{{Part: partStatus, Place: strconv.Itoa(e.Status), Rule: "not_in_spec"}}
		return c
	}
	for _, h := range rp.headers {
		c.Reply = append(c.Reply, h.check(h.values(r, nil, nil, e.ReplyHeader, nil))...)
	}
	if mt, _, _ := mime.ParseMediaType(e.ReplyHeader.Get("Content-Type")); mt == "text/event-stream" {
		if c.Skipped == "" {
			c.Skipped = "streaming"
		}
		return c
	}
	results, skipped = rp.body.check(e.ReplyHeader.Get("Content-Type"), e.ReplyBody)
	c.Reply = append(c.Reply, results...)
	if c.Skipped == "" {
		c.Skipped = skipped
	}
	return c
}

func (c *checks) reply(status int) *reply {
	for _, key := range []string{strconv.Itoa(status), fmt.Sprintf("%dXX", status/100), "default"} {
		if rp, ok := c.replies[key]; ok {
			return rp
		}
	}
	return nil
}

// values are the parameter's values in the call, nil when it is missing.
func (p param) values(r route, segments []string, query url.Values, header http.Header, cookies map[string]string) any {
	switch p.in {
	case "path":
		for i, segment := range r.segments {
			if segment == "{"+p.name+"}" {
				// The proxy only forwards paths that unescape.
				value, _ := url.PathUnescape(segments[i])
				return []string{value}
			}
		}
	case "query":
		if p.deepObject {
			return deepObject(p.name, query)
		}
		if values, ok := query[p.name]; ok {
			return values
		}
	case "header":
		if values := header.Values(p.name); len(values) > 0 {
			return []string{strings.Join(values, ", ")}
		}
	case "cookie":
		if value, ok := cookies[p.name]; ok {
			return []string{value}
		}
	}
	return nil
}

// deepObject reads name[key]=value pairs into an object, nil when there
// are none.
func deepObject(name string, query url.Values) any {
	found := url.Values{}
	for key, values := range query {
		if rest, ok := strings.CutPrefix(key, name+"["); ok {
			found["v["+rest] = values
		}
	}
	if len(found) == 0 {
		return nil
	}
	return nest(found)["v"]
}

func (p param) check(values any) []Result {
	part := p.in
	if values == nil {
		if p.required {
			return []Result{{Part: part, Place: p.name, Rule: "required"}}
		}
		return nil
	}
	if p.schema == nil {
		return nil
	}
	instance := values
	if list, ok := values.([]string); ok {
		instance = p.shape(list)
	}
	instance = coerce(instance, p.schema)
	return validate(p.schema, instance, part, p.name)
}

// shape turns the strings of a parameter into the value its schema wants:
// a list for an array, the first value otherwise.
func (p param) shape(list []string) any {
	if !slices.Contains(types(p.schema), "array") {
		return list[0]
	}
	if !p.explode || p.in != "query" {
		list = strings.Split(strings.Join(list, ","), ",")
	}
	items := make([]any, len(list))
	for i, v := range list {
		items[i] = v
	}
	return items
}

// unknownQuery reports query names the spec does not know. A login key
// sent in the query is known.
func (s *Spec) unknownQuery(params []param, query url.Values) []Result {
	var results []Result
	for _, key := range slices.Sorted(maps.Keys(query)) {
		if !s.secretQuery[key] && !knownQuery(params, key) {
			results = append(results, Result{Part: "query", Place: key, Rule: "not_in_spec"})
		}
	}
	return results
}

func knownQuery(params []param, key string) bool {
	for _, p := range params {
		if p.in == "query" && (p.name == key || p.deepObject && strings.HasPrefix(key, p.name+"[")) {
			return true
		}
	}
	return false
}

// loggedIn reports the credentials of the first way to log in when no way
// is complete. Only whether a credential is there is checked, never its
// value.
func (c *checks) loggedIn(header http.Header, query url.Values, cookies map[string]string) []Result {
	if len(c.security) == 0 {
		return nil
	}
	var first []Result
	for i, way := range c.security {
		var missing []Result
		for _, cred := range way {
			if !cred.present(header, query, cookies) {
				missing = append(missing, Result{Part: cred.in, Place: cred.name, Rule: "required"})
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if i == 0 {
			first = missing
		}
	}
	return first
}

func (cred credential) present(header http.Header, query url.Values, cookies map[string]string) bool {
	switch cred.in {
	case "query":
		return query.Has(cred.name)
	case "cookie":
		_, ok := cookies[cred.name]
		return ok
	}
	return header.Get(cred.name) != ""
}

// check checks a body against what the spec lists. A body that cannot be
// read, or is too large to check, is skipped, and the reason returned.
func (b *bodySpec) check(contentType string, body Body) ([]Result, string) {
	empty := len(body.Data) == 0 && !body.TooLarge
	switch {
	case b == nil && empty:
		return nil, ""
	case b == nil:
		return []Result{{Part: partBody, Place: whole, Rule: "not_in_spec"}}, ""
	case empty && b.required:
		return []Result{{Part: partBody, Place: whole, Rule: "required"}}, ""
	case empty:
		return nil, ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = ""
	}
	c := b.find(mediaType)
	if c == nil {
		listed := make([]string, len(b.content))
		for i, c := range b.content {
			listed[i] = c.mediaType
		}
		return []Result{{Part: partBody, Place: whole, Rule: "content_type", Expected: strings.Join(listed, ", ")}}, ""
	}
	var instance any
	switch {
	case isJSON(mediaType):
		if body.TooLarge {
			return nil, "body_too_large"
		}
		instance, err = jsonschema.UnmarshalJSON(bytes.NewReader(body.Data))
		if err != nil {
			return []Result{{Part: partBody, Place: whole, Rule: "json"}}, ""
		}
	case mediaType == "application/x-www-form-urlencoded":
		if body.TooLarge {
			return nil, "body_too_large"
		}
		values, err := url.ParseQuery(string(body.Data))
		if err != nil {
			return []Result{{Part: partBody, Place: whole, Rule: "form"}}, ""
		}
		instance = coerce(nest(values), c.schema)
	default:
		return nil, "unsupported_content_type"
	}
	if c.schema == nil {
		return nil, ""
	}
	return validate(c.schema, instance, partBody, whole), ""
}

// find picks the content for a media type: the exact type first, then
// type/* and */*.
func (b *bodySpec) find(mediaType string) *content {
	major, _, _ := strings.Cut(mediaType, "/")
	for _, want := range []string{mediaType, major + "/*", "*/*"} {
		for i, c := range b.content {
			if strings.EqualFold(c.mediaType, want) {
				return &b.content[i]
			}
		}
	}
	return nil
}

func isJSON(mediaType string) bool {
	// AWS's JSON protocol names its own versions of JSON.
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") ||
		mediaType == "application/x-amz-json-1.0" || mediaType == "application/x-amz-json-1.1"
}

// nest reads bracketed names, as Stripe writes them, into objects and
// lists: items[0][price]=p1 is {"items": [{"price": "p1"}]}. A name with no
// brackets keeps its first value.
func nest(values url.Values) map[string]any {
	root := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		path := splitBrackets(key)
		for _, value := range values[key] {
			put(root, path, value)
		}
	}
	return lists(root).(map[string]any)
}

func splitBrackets(key string) []string {
	name, rest, found := strings.Cut(key, "[")
	path := []string{name}
	for found {
		var part string
		part, rest, _ = strings.Cut(rest, "]")
		path = append(path, part)
		_, rest, found = strings.Cut(rest, "[")
	}
	return path
}

func put(m map[string]any, path []string, value string) {
	key := path[0]
	if key == "" {
		key = strconv.Itoa(len(m))
	}
	if len(path) == 1 {
		if _, set := m[key]; !set {
			m[key] = value
		}
		return
	}
	child, ok := m[key].(map[string]any)
	if !ok {
		child = map[string]any{}
		m[key] = child
	}
	put(child, path[1:], value)
}

// lists turns objects whose names are all numbers into lists, in the
// order of the numbers.
func lists(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for key, child := range m {
		m[key] = lists(child)
	}
	indexes := make([]int, 0, len(m))
	for key := range m {
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || strconv.Itoa(i) != key {
			return m
		}
		indexes = append(indexes, i)
	}
	if len(indexes) == 0 {
		return m
	}
	sort.Ints(indexes)
	list := make([]any, len(indexes))
	for i, index := range indexes {
		list[i] = m[strconv.Itoa(index)]
	}
	return list
}

// coerce turns text from a query, header, cookie or form into the number
// or true/false the schema wants, so the schema can check it.
func coerce(v any, s *jsonschema.Schema) any {
	if s == nil {
		return v
	}
	for s.Ref != nil && s.Types == nil && s.Properties == nil && s.Items2020 == nil {
		s = s.Ref
	}
	switch x := v.(type) {
	case string:
		return scalar(x, types(s))
	case []any:
		for i := range x {
			item := s.Items2020
			if i < len(s.PrefixItems) {
				item = s.PrefixItems[i]
			}
			x[i] = coerce(x[i], item)
		}
	case map[string]any:
		for key := range x {
			prop := s.Properties[key]
			if extra, ok := s.AdditionalProperties.(*jsonschema.Schema); ok && prop == nil {
				prop = extra
			}
			x[key] = coerce(x[key], prop)
		}
	}
	return v
}

func scalar(text string, want []string) any {
	switch {
	case slices.Contains(want, "integer") && isInteger(text),
		slices.Contains(want, "number") && isNumber(text):
		return jsonNumber(text)
	case slices.Contains(want, "boolean") && (text == "true" || text == "false"):
		return text == "true"
	}
	return text
}

func isInteger(text string) bool {
	_, ok := new(big.Int).SetString(text, 10)
	return ok
}

func isNumber(text string) bool {
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// types are the types a schema allows, following $ref.
func types(s *jsonschema.Schema) []string {
	for ; s != nil; s = s.Ref {
		if s.Types != nil {
			return s.Types.ToStrings()
		}
	}
	return nil
}

// validate checks a value against a schema and turns what is wrong into
// results. base names the value: a parameter's name, or $ for a body.
func validate(s *jsonschema.Schema, instance any, part, base string) []Result {
	var invalid *jsonschema.ValidationError
	if !errors.As(s.Validate(instance), &invalid) {
		return nil
	}
	var leaves []*jsonschema.ValidationError
	collect(invalid, &leaves)
	var results []Result
	for _, e := range leaves {
		place := placeOf(base, instance, e.InstanceLocation)
		switch k := e.ErrorKind.(type) {
		case *kind.Required:
			for _, name := range k.Missing {
				results = append(results, Result{Part: part, Place: place + "." + name, Rule: "required"})
			}
		case *kind.AdditionalProperties:
			for _, name := range k.Properties {
				results = append(results, Result{Part: part, Place: place + "." + name, Rule: "additionalProperties"})
			}
		case *kind.DependentRequired:
			for _, name := range k.Missing {
				results = append(results, Result{Part: part, Place: place + "." + name, Rule: "dependentRequired"})
			}
		case *kind.ContentMediaType:
			// A string that should hold JSON doesn't.
			results = append(results, Result{Part: part, Place: place, Rule: "json"})
		default:
			results = append(results, Result{Part: part, Place: place, Rule: rule(e.ErrorKind), Expected: expected(e.ErrorKind)})
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Place < results[j].Place })
	return results
}

// rule names the broken rule by its JSON Schema keyword.
func rule(k jsonschema.ErrorKind) string {
	switch k.(type) {
	case *kind.Not:
		return "not"
	case *kind.FalseSchema:
		return "false"
	}
	path := append([]string{"schema"}, k.KeywordPath()...)
	return path[len(path)-1]
}

// collect finds the errors that say what is wrong, leaving out those that
// only group others. For anyOf, oneOf and not the error itself is kept:
// the errors of each choice would only confuse.
func collect(e *jsonschema.ValidationError, leaves *[]*jsonschema.ValidationError) {
	switch e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf, *kind.Not:
		*leaves = append(*leaves, e)
		return
	}
	if len(e.Causes) == 0 {
		*leaves = append(*leaves, e)
		return
	}
	for _, cause := range e.Causes {
		collect(cause, leaves)
	}
}

// placeOf writes where a value sits: $.lines[1].sku in a body, tag[1] or
// filter.status in a parameter.
func placeOf(base string, instance any, location []string) string {
	var b strings.Builder
	b.WriteString(base)
	for _, token := range location {
		if token == insideJSON {
			// The value is JSON text; the place carries on inside it.
			if text, ok := instance.(string); ok {
				instance, _ = jsonschema.UnmarshalJSON(strings.NewReader(text))
			}
			continue
		}
		if list, ok := instance.([]any); ok {
			i, _ := strconv.Atoi(token)
			b.WriteString("[" + token + "]")
			instance = list[i]
			continue
		}
		b.WriteString("." + token)
		if m, ok := instance.(map[string]any); ok {
			instance = m[token]
		}
	}
	return b.String()
}

// expected is what the spec asks for, taken from the schema, never from
// the value.
func expected(k jsonschema.ErrorKind) string {
	switch k := k.(type) {
	case *kind.Type:
		return strings.Join(k.Want, " or ")
	case *kind.Format:
		return k.Want
	case *kind.Enum:
		want := make([]string, len(k.Want))
		for i, w := range k.Want {
			want[i] = fmt.Sprint(w)
		}
		return strings.Join(want, ", ")
	case *kind.Const:
		return fmt.Sprint(k.Want)
	case *kind.Pattern:
		return k.Want
	case *kind.Minimum:
		return k.Want.RatString()
	case *kind.Maximum:
		return k.Want.RatString()
	case *kind.ExclusiveMinimum:
		return k.Want.RatString()
	case *kind.ExclusiveMaximum:
		return k.Want.RatString()
	case *kind.MultipleOf:
		return k.Want.RatString()
	case *kind.MinLength:
		return strconv.Itoa(k.Want)
	case *kind.MaxLength:
		return strconv.Itoa(k.Want)
	case *kind.MinItems:
		return strconv.Itoa(k.Want)
	case *kind.MaxItems:
		return strconv.Itoa(k.Want)
	case *kind.MinProperties:
		return strconv.Itoa(k.Want)
	case *kind.MaxProperties:
		return strconv.Itoa(k.Want)
	}
	return ""
}
