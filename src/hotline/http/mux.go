package http

import (
	"fmt"
	"sort"
	"strings"
)

type RequestLocator struct {
	Method string
	Path   string
	Host   string
	Port   int
}

func (l *RequestLocator) Normalize() RequestLocator {
	return RequestLocator{
		Method: strings.ToUpper(l.Method),
		Path:   strings.ToLower(l.Path),
		Host:   strings.ToLower(l.Host),
		Port:   l.Port,
	}
}

type Mux[H any] struct {
	entries []patternEntry[H]
}
type patternEntry[H any] struct {
	pattern *RoutePattern
	handler *H
}

func (m *Mux[H]) LocaleHandler(locator RequestLocator) *H {
	for _, entry := range m.entries {
		if entry.pattern.Matches(locator) {
			return entry.handler
		}
	}
	return nil
}

func (m *Mux[H]) Add(route Route, handler *H) {
	pattern := NewRoutePattern(route)
	m.entries = append(m.entries, patternEntry[H]{
		pattern: pattern,
		handler: handler,
	})

	sort.Slice(m.entries, func(i, j int) bool {
		return len(m.entries[i].pattern.ID) > len(m.entries[j].pattern.ID)
	})
}

const UndefinedPort = 0

type Route struct {
	Method      string
	PathPattern string // /user/{user-id}/login/{session-id}/
	Host        string
	Port        int
}

func (r *Route) Normalize() Route {
	return Route{
		Method:      strings.ToUpper(r.Method),
		PathPattern: strings.ToLower(r.PathPattern),
		Host:        strings.ToLower(r.Host),
		Port:        r.Port,
	}
}

type RoutePattern struct {
	ID        string
	route     Route
	preParsed []pathPart
}

func NewRoutePattern(route Route) *RoutePattern {
	normalized := route.Normalize()
	parsed := parsePath(normalized.PathPattern)

	return &RoutePattern{
		ID:        fmt.Sprintf("%s:%s:%d:%s", normalized.Method, normalized.Host, normalized.Port, parsed.ID()),
		route:     normalized,
		preParsed: parsed,
	}
}

type pathPart struct {
	valueLower string
	isWildcard bool
	name       string // name of the wildcard (e.g., "user-id" for {user-id})
}

const urlDelimiter = "/"

type pathParts []pathPart

func (p pathParts) ID() string {
	id := ""
	for _, part := range p {
		if part.isWildcard {
			id += "/{}"
		} else {
			id += "/" + part.valueLower
		}
	}
	return id
}

func parsePath(pathPattern string) pathParts {
	normalized := normalizedPathParts(pathPattern)
	if len(normalized) == 0 {
		return pathParts{}
	}

	parts := make(pathParts, 0, len(normalized))
	for _, part := range normalized {
		isWildcard, name := parseWildcard(part)
		parts = append(parts, pathPart{
			valueLower: part,
			isWildcard: isWildcard,
			name:       name,
		})
	}
	return parts
}

func parseWildcard(part string) (bool, string) {
	if len(part) >= 2 && part[0] == '{' && part[len(part)-1] == '}' {
		name := part[1 : len(part)-1]
		return true, name
	}
	return false, ""
}

func (p *RoutePattern) Matches(locator RequestLocator) bool {
	locator = locator.Normalize()
	if p.route.Method != locator.Method {
		return false
	}

	if p.route.Host != "" && p.route.Host != locator.Host {
		return false
	}

	if p.route.Port != UndefinedPort && p.route.Port != locator.Port {
		return false
	}

	return p.matchesPath(locator.Path)
}

func (p *RoutePattern) matchesPath(path string) bool {
	if len(p.preParsed) == 0 {
		return path == "/"
	}

	pathParts := normalizedPathParts(path)

	if len(pathParts) < len(p.preParsed) {
		return false
	}

	for i, part := range p.preParsed {
		if part.isWildcard {
			continue
		}

		if part.valueLower != pathParts[i] {
			return false
		}
	}
	return true
}

func normalizedPathParts(path string) []string {
	var pathParts []string
	parts := strings.Split(strings.Trim(path, urlDelimiter), urlDelimiter)
	pathParts = make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) != 0 {
			pathParts = append(pathParts, part)
		}
	}
	return pathParts
}
