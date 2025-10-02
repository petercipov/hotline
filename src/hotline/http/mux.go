package http

import (
	"encoding/base64"
	"fmt"
	"hash/crc64"
	"iter"
	"sort"
	"strconv"
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
	route   Route
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

func (m *Mux[H]) Upsert(route Route, handler *H) {
	for index, entry := range m.entries {
		if entry.route == route {
			m.entries[index].handler = handler
			return
		}
	}

	pattern := NewRoutePattern(route)
	m.entries = append(m.entries, patternEntry[H]{
		route:   route,
		pattern: pattern,
		handler: handler,
	})

	sort.SliceStable(m.entries, func(i, j int) bool {
		return len(m.entries[i].pattern.ID) > len(m.entries[j].pattern.ID)
	})
}

func (m *Mux[H]) Handlers() iter.Seq2[Route, H] {
	return func(yield func(Route, H) bool) {
		for _, entry := range m.entries {
			if !yield(entry.route, *entry.handler) {
				break
			}
		}
	}
}

func (m *Mux[H]) Delete(route Route) {
	for index, entry := range m.entries {
		if entry.route == route {
			m.entries = append(m.entries[:index], m.entries[index+1:]...)
			return
		}
	}
}

const AnyPort = 0
const AnyMethod = ""
const AnyHost = ""

type Route struct {
	Method      string
	PathPattern string // /user/{user-id}/login/{session-id}/
	Host        string
	Port        int
}

type RouteKey string

func (k *RouteKey) String() string {
	return string(*k)
}

func (r *Route) Normalize() Route {
	return Route{
		Method:      strings.ToUpper(r.Method),
		PathPattern: strings.ToLower(r.PathPattern),
		Host:        strings.ToLower(r.Host),
		Port:        r.Port,
	}
}

func (r *Route) GenerateKey(salt string) RouteKey {
	table := crc64.MakeTable(crc64.ISO)
	hash := crc64.New(table)

	_, _ = hash.Write([]byte(salt))
	_, _ = hash.Write([]byte(":"))
	_, _ = hash.Write([]byte(r.Method))
	_, _ = hash.Write([]byte(":"))
	_, _ = hash.Write([]byte(r.Host))
	_, _ = hash.Write([]byte(":"))
	if r.Port == AnyPort {
		_, _ = hash.Write([]byte(strconv.Itoa(r.Port)))
	}
	_, _ = hash.Write([]byte(":"))
	_, _ = hash.Write([]byte(r.PathPattern))
	_, _ = hash.Write([]byte(":"))
	_, _ = hash.Write([]byte(salt))

	keyBytes := hash.Sum(nil)
	return RouteKey("RK" + base64.RawURLEncoding.EncodeToString(keyBytes))
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
	name       string // name of the wildcard (e.g., "user-id" for {user-id})
	isWildcard bool
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
	if p.route.Method != AnyMethod && p.route.Method != locator.Method {
		return false
	}

	if p.route.Host != AnyHost && p.route.Host != locator.Host {
		return false
	}

	if p.route.Port != AnyPort && p.route.Port != locator.Port {
		return false
	}

	return p.matchesPath(locator.Path)
}

func (p *RoutePattern) matchesPath(path string) bool {
	if len(p.preParsed) == 0 {
		return true
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
