// Package overlay applies OpenAPI Overlay 1.1 files to a spec, and makes an
// overlay that turns one spec into another. Targets are always read as
// standard JSONPath (RFC 9535).
package overlay

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/speakeasy-api/openapi/overlay"
	"go.yaml.in/yaml/v3"
)

// ErrInvalid marks an overlay, or a spec given to Make, the proxy cannot use.
var ErrInvalid = errors.New("overlay refused")

// errAction marks a problem with one action, named by its number.
var errAction = errors.New("action")

// File is one overlay file: the name problems name it by, and its content.
type File struct {
	Name string
	Data []byte
}

// Apply applies the overlays to the spec, in order, and returns the merged
// spec. An action whose target matches nothing does nothing, as the format
// says, and gets a warning instead.
func Apply(spec []byte, files []File) ([]byte, []string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(spec, &root); err != nil {
		return nil, nil, fmt.Errorf("%w: the spec is not YAML or JSON: %w", ErrInvalid, err)
	}
	var warnings []string
	for _, f := range files {
		o, err := parse(f)
		if err != nil {
			return nil, nil, err
		}
		w, err := apply(o, &root)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s, %w", ErrInvalid, f.Name, err)
		}
		for _, warning := range w {
			warnings = append(warnings, fmt.Sprintf("overlay %s, %s", f.Name, warning))
		}
	}
	merged, err := yaml.Marshal(&root)
	return merged, warnings, err
}

// parse reads an overlay file, which must be version 1.1.
func parse(f File) (*overlay.Overlay, error) {
	o, err := overlay.ParseReader(bytes.NewReader(f.Data))
	if err != nil || o.Version == "" {
		return nil, fmt.Errorf("%w: %s is not an overlay document", ErrInvalid, f.Name)
	}
	if !strings.HasPrefix(o.Version, "1.1.") {
		return nil, fmt.Errorf("%w: %s: version %s is not supported, only 1.1", ErrInvalid, f.Name, o.Version)
	}
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s is not an overlay document: %s", ErrInvalid, f.Name, strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	// A file may ask the library for its older JSONPath; the format allows
	// only the standard one.
	o.JSONPathVersion = overlay.JSONPathRFC9535
	return o, nil
}

// apply runs the overlay's actions on root one by one, each on the result
// of the one before. It returns a warning for each action that matches
// nothing.
func apply(o *overlay.Overlay, root *yaml.Node) ([]string, error) {
	var warnings []string
	for i, action := range o.Actions {
		n := i + 1
		targets, err := query(o, root, action.Target)
		if err != nil {
			return nil, fmt.Errorf("%w %d: target %w", errAction, n, err)
		}
		if len(targets) == 0 {
			warnings = append(warnings, fmt.Sprintf("action %d: target %s matches nothing, so it does nothing", n, action.Target))
			continue
		}
		// remove wins over update, and update over copy, as the format says.
		if action.Copy != "" && !action.Remove && action.Update.IsZero() {
			sources, err := query(o, root, action.Copy)
			if err != nil {
				return nil, fmt.Errorf("%w %d: copy %w", errAction, n, err)
			}
			if len(sources) != 1 {
				return nil, fmt.Errorf("%w %d: copy %s matches %s", errAction, n, action.Copy, spots(len(sources)))
			}
		}
		one := *o
		one.Actions = []overlay.Action{action}
		if _, err := one.ApplyToStrict(root); err != nil {
			return nil, fmt.Errorf("%w %d: %w", errAction, n, err)
		}
	}
	return warnings, nil
}

func query(o *overlay.Overlay, root *yaml.Node, path string) ([]*yaml.Node, error) {
	p, err := o.NewPath(path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSONPath: %w", path, err)
	}
	return p.Query(root), nil
}

func spots(n int) string {
	if n == 0 {
		return "nothing"
	}
	return fmt.Sprintf("%d spots, not one", n)
}

// Make returns an overlay, version 1.1, that turns the spec from into the
// spec to. It applies the overlay to from before returning it, and refuses
// one that does not give to.
func Make(title string, from, to File) ([]byte, error) {
	var a, b, check yaml.Node
	for _, f := range []struct {
		file File
		node *yaml.Node
	}{{from, &a}, {to, &b}, {from, &check}} {
		if err := yaml.Unmarshal(f.file.Data, f.node); err != nil {
			return nil, fmt.Errorf("%w: %s is not YAML or JSON: %w", ErrInvalid, f.file.Name, err)
		}
	}
	// Compare makes no error of its own: it only passes on those of its
	// walk through the two documents, which has none either.
	o, err := overlay.Compare(title, &a, b)
	if err != nil || len(o.Actions) == 0 {
		return nil, fmt.Errorf("%w: the two specs are the same, so there is nothing to overlay", ErrInvalid)
	}
	o.Version = overlay.Version110
	o.JSONPathVersion = ""
	o.Info.Version = "1"
	if _, err := apply(o, &check); err != nil || !same(&check, &b) {
		return nil, fmt.Errorf("%w: the overlay made from %s does not turn it into %s, so it is not written", ErrInvalid, from.Name, to.Name)
	}
	out, err := o.ToString()
	return []byte(out), err
}

// same says whether two documents hold the same values, whatever the order
// of their keys.
func same(a, b *yaml.Node) bool {
	var x, y any
	_ = a.Decode(&x)
	_ = b.Decode(&y)
	return reflect.DeepEqual(x, y)
}
