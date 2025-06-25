package http

import (
	"strings"
)

type Route struct {
	Method      string
	PathPattern string // /user/{user-id}/login/{session-id}/
}

func (r *Route) Normalize() Route {
	return Route{
		Method:      strings.ToUpper(r.Method),
		PathPattern: strings.ToLower(r.PathPattern),
	}
}

type RoutePattern struct {
	route Route

	preParsed []pathPart
}

func NewRoutePattern(route Route) *RoutePattern {
	return &RoutePattern{
		route:     route.Normalize(),
		preParsed: parsePath(route.PathPattern),
	}
}

type pathPart struct {
	valueLower string
	isWildcard bool
	name       string // name of the wildcard (e.g., "user-id" for {user-id})
}

const urlDelimiter = "/"

func parsePath(pathPattern string) []pathPart {
	parts := normalizedPathParts(pathPattern)
	if len(parts) == 0 {
		return []pathPart{}
	}

	pathParts := make([]pathPart, 0, len(parts))
	for _, part := range parts {
		isWildcard, name := parseWildcard(part)
		pathParts = append(pathParts, pathPart{
			valueLower: part,
			isWildcard: isWildcard,
			name:       name,
		})
	}

	return pathParts
}

func parseWildcard(part string) (bool, string) {
	if len(part) >= 2 && part[0] == '{' && part[len(part)-1] == '}' {
		name := part[1 : len(part)-1]
		return true, name
	}
	return false, ""
}

func (p *RoutePattern) Matches(method, path string) bool {
	if p.route.Method != strings.ToUpper(method) {
		return false
	}

	return p.matchesPath(path)
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
			pathParts = append(pathParts, strings.ToLower(part))
		}
	}
	return pathParts
}
