package spec

import (
	"encoding/json"
	"mime"
	"net/url"
)

// NamesActionInBody says whether the spec picks routes by a field of the
// request body (x-hotline-action-field), so the route is known only once
// the whole request body has arrived.
func (s *Spec) NamesActionInBody() bool {
	return s.actionField != ""
}

// BodyAction reads the action from a request body, for a spec that names
// it in a body field: the field's value at the top level of a form or JSON
// body. It is empty when the spec names no field, the body is larger than
// the copy, is neither form nor JSON, or has no such field as text.
func (s *Spec) BodyAction(contentType string, body Body) string {
	if s.actionField == "" || body.TooLarge {
		return ""
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch {
	case mediaType == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body.Data))
		if err != nil {
			return ""
		}
		return values.Get(s.actionField)
	case isJSON(mediaType):
		var fields map[string]json.RawMessage
		if json.Unmarshal(body.Data, &fields) != nil {
			return ""
		}
		var action string
		if json.Unmarshal(fields[s.actionField], &action) != nil {
			return ""
		}
		return action
	}
	return ""
}
