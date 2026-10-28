// Package spec reads our copy of a vendor's OpenAPI spec: its route
// templates, its x-hotline-redact marks and the secrets named in its
// security section. It uses them to redact the vendor address and to pick
// the headers a record may hold.
package spec

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ErrInvalid marks a spec the proxy cannot use.
var ErrInvalid = errors.New("spec: invalid")

// Spec is a read, checked vendor spec. It never changes after Parse.
type Spec struct {
	routes        []route
	secretHeaders map[string]bool
	secretQuery   map[string]bool
}

// mark is what an x-hotline-redact mark says about one value.
type mark int

const (
	unmarked mark = iota
	keep
	redact
)

type route struct {
	method   string
	template string
	segments []string
	// marks hold the parameter marks by where the parameter lives
	// ("path", "query", "header" and "reply") and then by name. Header
	// names are lower case.
	marks map[string]map[string]mark
}

type document struct {
	OpenAPI string `yaml:"openapi"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths      map[string]pathItem `yaml:"paths"`
	Components struct {
		SecuritySchemes map[string]securityScheme `yaml:"securitySchemes"`
	} `yaml:"components"`
}

type pathItem struct {
	Parameters []parameter `yaml:"parameters"`
	Get        *operation  `yaml:"get"`
	Put        *operation  `yaml:"put"`
	Post       *operation  `yaml:"post"`
	Delete     *operation  `yaml:"delete"`
	Options    *operation  `yaml:"options"`
	Head       *operation  `yaml:"head"`
	Patch      *operation  `yaml:"patch"`
	Trace      *operation  `yaml:"trace"`
}

type operation struct {
	Parameters []parameter         `yaml:"parameters"`
	Responses  map[string]response `yaml:"responses"`
}

type response struct {
	Headers map[string]struct {
		Redact any `yaml:"x-hotline-redact"` //nolint:tagliatelle // the name of our OpenAPI extension
	} `yaml:"headers"`
}

type parameter struct {
	Name   string `yaml:"name"`
	In     string `yaml:"in"`
	Redact any    `yaml:"x-hotline-redact"` //nolint:tagliatelle // the name of our OpenAPI extension
}

type securityScheme struct {
	Type string `yaml:"type"`
	In   string `yaml:"in"`
	Name string `yaml:"name"`
}

// Parse reads and checks a spec. name only labels the problems it reports.
// Every problem is listed, not only the first one.
func Parse(name string, data []byte) (*Spec, error) {
	var doc document
	err := yaml.Unmarshal(data, &doc)
	if err != nil || !strings.HasPrefix(doc.OpenAPI, "3.") {
		return nil, fmt.Errorf("%w: %s is not a valid OpenAPI 3 document", ErrInvalid, name)
	}
	s := &Spec{secretHeaders: map[string]bool{}, secretQuery: map[string]bool{}}
	var problems []error
	// A base path in the first servers address, such as /v1 in
	// https://api.x.com/v1, goes in front of every route.
	base := ""
	if len(doc.Servers) > 0 {
		if u, err := url.Parse(doc.Servers[0].URL); err == nil {
			base = strings.TrimSuffix(u.Path, "/")
		}
	}
	for template, item := range doc.Paths {
		template = base + template
		for method, op := range item.operations() {
			r := route{
				method:   method,
				template: template,
				segments: strings.Split(strings.TrimSuffix(template, "/"), "/"),
				marks:    map[string]map[string]mark{"path": {}, "query": {}, "header": {}, "reply": {}},
			}
			where := fmt.Sprintf("%s %s %s", name, method, template)
			// The method's own entries come last, so they win.
			for _, p := range append(append([]parameter{}, item.Parameters...), op.Parameters...) {
				problems = r.addMark(problems, where, p.In, p.Name, p.Redact)
			}
			for _, reply := range op.Responses {
				for header, h := range reply.Headers {
					problems = r.addMark(problems, where, "reply", header, h.Redact)
				}
			}
			s.routes = append(s.routes, r)
		}
	}
	for _, scheme := range doc.Components.SecuritySchemes {
		switch {
		case scheme.Type == "apiKey" && scheme.In == "header":
			s.secretHeaders[strings.ToLower(scheme.Name)] = true
		case scheme.Type == "apiKey" && scheme.In == "query":
			s.secretQuery[scheme.Name] = true
		}
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return s, nil
}

func (p pathItem) operations() map[string]*operation {
	all := map[string]*operation{
		http.MethodGet: p.Get, http.MethodPut: p.Put, http.MethodPost: p.Post,
		http.MethodDelete: p.Delete, http.MethodOptions: p.Options, http.MethodHead: p.Head,
		http.MethodPatch: p.Patch, http.MethodTrace: p.Trace,
	}
	for method, op := range all {
		if op == nil {
			delete(all, method)
		}
	}
	return all
}

func (r route) addMark(problems []error, where, in, name string, value any) []error {
	var m mark
	switch value {
	case nil:
		return problems
	case true:
		m = redact
	case false:
		m = keep
	default:
		return append(problems, fmt.Errorf("%w: %s: x-hotline-redact on %s must be true or false", ErrInvalid, where, name))
	}
	if in != "path" && in != "query" {
		name = strings.ToLower(name)
	}
	// A cookie parameter has no entry: cookies are never recorded.
	if marks, ok := r.marks[in]; ok {
		marks[name] = m
	}
	return problems
}
