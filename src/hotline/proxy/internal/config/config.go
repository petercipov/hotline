// Package config reads and checks hotline-proxy.yaml and the specs it points
// to. A config is either valid in full or refused with every problem listed.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"hotline/proxy/internal/spec"
)

// ErrInvalid marks a config the proxy refuses.
var ErrInvalid = errors.New("config: invalid")

// Config is a loaded, checked config. It never changes after Load.
type Config struct {
	ProxyAddr string
	Exporters []Exporter
	// NotPassed applies to calls without a known integration ID, and to
	// calls to a host outside the integration's apis.
	NotPassed Settings
	// Version is the config's own version field, carried by every record.
	Version string
	// Warnings name settings the proxy does not use, such as a misspelt
	// field. They do not stop the proxy.
	Warnings     []string
	integrations map[string]map[string]*API
}

// Exporter is one OTLP destination.
type Exporter struct {
	Endpoint string
	Delta    bool
	// QueueSize is how many records wait for the destination at most.
	QueueSize int
}

// Settings say how a call is made.
type Settings struct {
	TLSOrigination bool
	VendorTimeout  time.Duration
}

// API is one vendor api of an integration.
type API struct {
	Settings
	Spec *spec.Spec
}

// API finds the api an integration uses for a host, or nil when the ID is
// unknown or the host is outside the integration's apis.
func (c *Config) API(integrationID, host string) *API {
	return c.integrations[integrationID][strings.ToLower(host)]
}

// file is the config file's layout. Settings that later slices use, such
// as telemetry, are listed so that they do not count as unknown.
type file struct {
	Version string `yaml:"version"`
	Listen  struct {
		Proxy string `yaml:"proxy"`
		Admin any    `yaml:"admin"`
		TLS   any    `yaml:"tls"`
	} `yaml:"listen"`
	Telemetry any `yaml:"telemetry"`
	Exporters []struct {
		OTLP struct {
			Endpoint    string `yaml:"endpoint"`
			Temporality string `yaml:"temporality"`
			QueueSize   *int   `yaml:"queueSize"`
		} `yaml:"otlp"`
	} `yaml:"exporters"`
	Integrations []struct {
		ID             string  `yaml:"id"`
		FingerprintKey any     `yaml:"fingerprintKey"`
		TLSOrigination *bool   `yaml:"tlsOrigination"`
		VendorTimeout  *string `yaml:"vendorTimeout"`
		APIs           []struct {
			Hosts          []string `yaml:"hosts"`
			Spec           string   `yaml:"spec"`
			TLSOrigination *bool    `yaml:"tlsOrigination"`
			VendorTimeout  *string  `yaml:"vendorTimeout"`
		} `yaml:"apis"`
	} `yaml:"integrations"`
}

const notPassed = "not-passed"

// Load reads the config file and every spec it points to. Spec paths are
// relative to the folder of the config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator chooses the config file
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%w: %s is not valid YAML: %w", ErrInvalid, path, err)
	}
	var warnings []string
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	var unknown *yaml.TypeError
	if errors.As(strict.Decode(&file{}), &unknown) {
		for _, e := range unknown.Errors {
			field, _, _ := strings.Cut(e, " not found")
			warnings = append(warnings, field+" is not a setting and is ignored")
		}
	}
	l := loader{
		dir:   filepath.Dir(path),
		specs: map[string]*spec.Spec{},
	}
	c := &Config{
		Version:      f.Version,
		Warnings:     warnings,
		ProxyAddr:    f.Listen.Proxy,
		NotPassed:    Settings{VendorTimeout: time.Minute},
		integrations: map[string]map[string]*API{},
	}
	if f.Version == "" {
		l.problem("there is no version")
	}
	if len(f.Exporters) == 0 {
		l.problem("there are no exporters")
	}
	for _, e := range f.Exporters {
		switch e.OTLP.Temporality {
		case "", "cumulative":
		case "delta":
		default:
			l.problem("exporter %s: temporality must be cumulative or delta", e.OTLP.Endpoint)
		}
		exporter := Exporter{Endpoint: e.OTLP.Endpoint, Delta: e.OTLP.Temporality == "delta", QueueSize: 10_000}
		if e.OTLP.QueueSize != nil {
			exporter.QueueSize = *e.OTLP.QueueSize
		}
		if exporter.QueueSize < 1 {
			l.problem("exporter %s: queueSize must be at least 1", e.OTLP.Endpoint)
		}
		c.Exporters = append(c.Exporters, exporter)
	}
	for _, in := range f.Integrations {
		if in.ID == notPassed && len(in.APIs) > 0 {
			c.Warnings = append(c.Warnings, "the not-passed entry applies to every host, so its apis list is ignored")
		}
		if in.ID == notPassed {
			c.NotPassed = l.settings(in.ID, in.TLSOrigination, in.VendorTimeout, c.NotPassed)
			continue
		}
		if _, seen := c.integrations[in.ID]; seen {
			l.problem("two integrations have the ID %s", in.ID)
		}
		apis := map[string]*API{}
		for _, a := range in.APIs {
			api := &API{
				Settings: l.settings(in.ID, a.TLSOrigination, a.VendorTimeout, Settings{TLSOrigination: true, VendorTimeout: time.Minute}),
				Spec:     l.spec(in.ID, a.Spec),
			}
			for _, host := range a.Hosts {
				host = strings.ToLower(host)
				if _, seen := apis[host]; seen {
					l.problem("integration %s lists the host %s in two apis", in.ID, host)
				}
				apis[host] = api
			}
		}
		c.integrations[in.ID] = apis
	}
	if len(l.problems) > 0 {
		return nil, errors.Join(l.problems...)
	}
	return c, nil
}

type loader struct {
	dir      string
	specs    map[string]*spec.Spec
	problems []error
}

func (l *loader) problem(format string, args ...any) {
	l.problems = append(l.problems, fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...)))
}

func (l *loader) settings(id string, tlsOrigination *bool, vendorTimeout *string, s Settings) Settings {
	if tlsOrigination != nil {
		s.TLSOrigination = *tlsOrigination
	}
	if vendorTimeout != nil {
		d, err := time.ParseDuration(*vendorTimeout)
		if err != nil || d <= 0 {
			l.problem("integration %s: vendorTimeout %q must be a duration above zero, such as 10s", id, *vendorTimeout)
		}
		s.VendorTimeout = d
	}
	return s
}

func (l *loader) spec(id, path string) *spec.Spec {
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.dir, path)
	}
	if s, read := l.specs[path]; read {
		return s
	}
	data, err := os.ReadFile(path) //nolint:gosec // the config names its spec files
	if err != nil {
		l.problem("integration %s: spec %s cannot be read: %v", id, path, err)
		return nil
	}
	s, err := spec.Parse(path, data)
	if err != nil {
		l.problems = append(l.problems, err)
	}
	l.specs[path] = s
	return s
}
