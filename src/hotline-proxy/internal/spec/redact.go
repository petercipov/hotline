package spec

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Call is what the proxy knows about a call when it builds the record.
type Call struct {
	Method string
	// Origin is the vendor address up to the path, such as
	// https://api.talon.one or http://api.x.com:8080.
	Origin       string
	EscapedPath  string
	RawQuery     string
	Header       http.Header
	ReplyHeaders http.Header
}

// Redacted is the part of a record that the spec decides. URLFull and
// Template are empty when the spec has no route for the call: without a
// route, the proxy cannot tell values from fixed parts of the path.
type Redacted struct {
	URLFull      string
	Template     string
	Header       map[string][]string
	ReplyHeaders map[string][]string
}

const redacted = "REDACTED"

// Redact applies the spec's marks and secrets to a call. Path and query
// values are redacted unless marked x-hotline-redact false. Headers are kept
// only when on the default list or marked; secrets never are.
func (s *Spec) Redact(c Call) Redacted {
	r := s.match(c.Method, c.EscapedPath, c.Header)
	redacted := Redacted{
		Header:       s.headers(c.Header, r.marks["header"]),
		ReplyHeaders: s.headers(c.ReplyHeaders, r.marks["reply"]),
	}
	if r.segments == nil {
		return redacted
	}
	path := strings.TrimSuffix(c.EscapedPath, "/")
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = r.pathValue(i, segment)
	}
	redacted.URLFull = c.Origin + strings.Join(segments, "/") + strings.TrimPrefix(c.EscapedPath, path)
	if c.RawQuery != "" {
		redacted.URLFull += "?" + s.redactQuery(r, c.RawQuery)
	}
	redacted.Template = r.template
	return redacted
}

// match finds the route for a call. A fixed segment wins over a templated
// one, and a trailing slash makes no difference. A route named by the
// action header matches only when the header has exactly its value, and
// wins over the same path without one. With no route, it returns a route
// with no segments and no marks.
func (s *Spec) match(method, escapedPath string, header http.Header) route {
	segments := strings.Split(strings.TrimSuffix(escapedPath, "/"), "/")
	action := ""
	if s.actionHeader != "" {
		action = header.Get(s.actionHeader)
	}
	best := route{marks: map[string]map[string]mark{}}
	for _, r := range s.routes {
		if r.method != method || !r.fits(segments) || r.action != "" && r.action != action {
			continue
		}
		if r.beats(best) || r.action != "" && best.action == "" && slices.Equal(r.segments, best.segments) {
			best = r
		}
	}
	return best
}

func (r route) fits(segments []string) bool {
	if len(r.segments) != len(segments) {
		return false
	}
	for i, want := range r.segments {
		if !isValue(want) && want != segments[i] || isValue(want) && segments[i] == "" {
			return false
		}
	}
	return true
}

// beats says whether r is more specific than other: at the first segment
// where they differ, r has a fixed segment and other a value.
func (r route) beats(other route) bool {
	for i, segment := range other.segments {
		if isValue(segment) != isValue(r.segments[i]) {
			return isValue(segment)
		}
	}
	return true
}

func (r route) pathValue(i int, segment string) string {
	template := r.segments[i]
	if !isValue(template) || r.marks["path"][strings.Trim(template, "{}")] == keep {
		return segment
	}
	return redacted
}

func isValue(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func (s *Spec) redactQuery(r route, rawQuery string) string {
	pairs := strings.Split(rawQuery, "&")
	for i, pair := range pairs {
		name, value, _ := strings.Cut(pair, "=")
		// A name that does not unescape matches no mark, so it is redacted.
		plain, _ := url.QueryUnescape(name)
		// An empty value has nothing to hide, so it stays empty.
		if value != "" && (s.secretQuery[plain] || r.marks["query"][plain] != keep) {
			pairs[i] = name + "=" + redacted
		}
	}
	return strings.Join(pairs, "&")
}

func (s *Spec) headers(h http.Header, marks map[string]mark) map[string][]string {
	kept := map[string][]string{}
	for name, values := range h {
		name = strings.ToLower(name)
		switch {
		case neverRecorded(name) || s.secretHeaders[name]:
		case marks[name] == redact:
			kept[name] = []string{redacted}
		case marks[name] == keep || onDefaultList(name):
			kept[name] = values
		}
	}
	return kept
}

func neverRecorded(name string) bool {
	switch name {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	return false
}

func onDefaultList(name string) bool {
	switch name {
	case "content-type", "content-length", "date", "retry-after", "request-id", "x-request-id":
		return true
	}
	return strings.HasPrefix(name, "ratelimit-")
}
