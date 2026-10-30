// Package config reads and checks hotline-proxy.yaml and the specs it points
// to. A config is either valid in full or refused with every problem listed.
package config

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"hotline-proxy/internal/spec"
)

// ErrInvalid marks a config the proxy refuses.
var ErrInvalid = errors.New("config: invalid")

// Config is a loaded, checked config. It never changes after Load.
type Config struct {
	ProxyAddr string
	// TLS, when set, makes the proxy port accept only TLS, with this
	// certificate.
	TLS *TLS
	// Connect, when set, makes CONNECT an ingress: this issuer signs the
	// certificate the proxy shows for each vendor host inside a tunnel.
	Connect   *TLS
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

// TLS is a certificate the proxy shows apps, or the issuer it signs them
// with: the files it comes from, and the pair they held when the config
// was loaded.
type TLS struct {
	CertFile    string
	KeyFile     string
	Certificate *tls.Certificate
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
	// AllowInternalIPs lets calls go to loopback, private and other
	// internal IPs, which the proxy otherwise refuses.
	AllowInternalIPs bool
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
		Proxy   string    `yaml:"proxy"`
		Admin   any       `yaml:"admin"`
		TLS     *pairFile `yaml:"tls"`
		Connect *pairFile `yaml:"connect"`
	} `yaml:"listen"`
	Telemetry any `yaml:"telemetry"`
	// APIs are the vendor apis, each written once and named by the
	// integrations that use it.
	APIs map[string]struct {
		Hosts    []string     `yaml:"hosts"`
		Spec     string       `yaml:"spec"`
		Settings settingsFile `yaml:",inline"`
	} `yaml:"apis"`
	Exporters []struct {
		OTLP struct {
			Endpoint    string `yaml:"endpoint"`
			Temporality string `yaml:"temporality"`
			QueueSize   *int   `yaml:"queueSize"`
		} `yaml:"otlp"`
	} `yaml:"exporters"`
	Integrations []struct {
		ID             string       `yaml:"id"`
		FingerprintKey any          `yaml:"fingerprintKey"`
		Settings       settingsFile `yaml:",inline"`
		APIs           []apiRef     `yaml:"apis"`
	} `yaml:"integrations"`
}

// pairFile names a certificate file and its key file.
type pairFile struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// apiRef is an api an integration names: just its name, or
// { api: name, vendorTimeout: 5s } to change how long its calls wait.
type apiRef struct {
	name          string
	vendorTimeout *string
	// unknown are the lines of settings an integration may not change.
	unknown []string
}

func (r *apiRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		r.name = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: line %d: an api is named by its name or by { api: name }", ErrInvalid, node.Line)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "api":
			r.name = value.Value
		case "vendorTimeout":
			r.vendorTimeout = &value.Value
		default:
			r.unknown = append(r.unknown, fmt.Sprintf("line %d: field %s", key.Line, key.Value))
		}
	}
	return nil
}

// settingsFile holds the settings an api or the not-passed entry may set.
type settingsFile struct {
	TLSOrigination   *bool   `yaml:"tlsOrigination"`
	VendorTimeout    *string `yaml:"vendorTimeout"`
	AllowInternalIPs *bool   `yaml:"allowInternalIPs"`
}

const notPassed = "not-passed"

// Load reads the config file and every spec and certificate it points to.
// Their paths are relative to the folder of the config file. A certificate
// that has expired by now is refused.
func Load(path string, now time.Time) (*Config, error) {
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
	if f.Listen.Proxy == "" {
		l.problem("there is no listen.proxy address")
	}
	if t := f.Listen.TLS; t != nil {
		c.TLS = l.pair("listen.tls", t, LoadCertificate, now)
	}
	if t := f.Listen.Connect; t != nil {
		c.Connect = l.pair("listen.connect", t, LoadIssuer, now)
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
	apis := map[string]*API{}
	hosts := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(f.APIs)) {
		a := f.APIs[name]
		label := "api " + name
		if a.Spec == "" {
			l.problem("%s has no spec", label)
		}
		apis[name] = &API{
			Settings: l.settings(label, a.Settings, Settings{TLSOrigination: true, VendorTimeout: time.Minute}),
			Spec:     l.spec(label, a.Spec),
		}
		for _, host := range a.Hosts {
			hosts[name] = append(hosts[name], strings.ToLower(host))
		}
	}
	for _, in := range f.Integrations {
		if in.ID == notPassed && len(in.APIs) > 0 {
			c.Warnings = append(c.Warnings, "the not-passed entry applies to every host, so its apis list is ignored")
		}
		if in.ID == "" {
			l.problem("an integration has no id")
			continue
		}
		if in.ID == notPassed {
			c.NotPassed = l.settings("integration "+in.ID, in.Settings, c.NotPassed)
			continue
		}
		if _, seen := c.integrations[in.ID]; seen {
			l.problem("two integrations have the ID %s", in.ID)
		}
		byHost := map[string]*API{}
		for _, ref := range in.APIs {
			for _, u := range ref.unknown {
				c.Warnings = append(c.Warnings, u+" is not a setting an integration may change, and is ignored")
			}
			api, known := apis[ref.name]
			if !known {
				l.problem("integration %s names the api %s, which is not in apis", in.ID, ref.name)
				continue
			}
			if ref.vendorTimeout != nil {
				changed := *api
				changed.Settings = l.settings("integration "+in.ID, settingsFile{VendorTimeout: ref.vendorTimeout}, api.Settings)
				api = &changed
			}
			for _, host := range hosts[ref.name] {
				if _, seen := byHost[host]; seen {
					l.problem("integration %s names two apis with the host %s", in.ID, host)
				}
				byHost[host] = api
			}
		}
		c.integrations[in.ID] = byHost
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

func (l *loader) settings(label string, f settingsFile, s Settings) Settings {
	if f.TLSOrigination != nil {
		s.TLSOrigination = *f.TLSOrigination
	}
	if f.VendorTimeout != nil {
		d, err := time.ParseDuration(*f.VendorTimeout)
		if err != nil || d <= 0 {
			l.problem("%s: vendorTimeout %q must be a duration above zero, such as 10s", label, *f.VendorTimeout)
		}
		s.VendorTimeout = d
	}
	if f.AllowInternalIPs != nil {
		s.AllowInternalIPs = *f.AllowInternalIPs
	}
	return s
}

func (l *loader) pair(label string, f *pairFile, load func(certFile, keyFile string, now time.Time) (*tls.Certificate, error), now time.Time) *TLS {
	if f.Cert == "" || f.Key == "" {
		l.problem("%s needs both cert and key", label)
		return nil
	}
	t := &TLS{CertFile: l.path(f.Cert), KeyFile: l.path(f.Key)}
	cert, err := load(t.CertFile, t.KeyFile, now)
	if err != nil {
		l.problems = append(l.problems, err)
	}
	t.Certificate = cert
	return t
}

// LoadCertificate reads a PEM certificate, with any issuers in between, and
// its key, and checks that they match and that the certificate has not
// expired by now.
func LoadCertificate(certFile, keyFile string, now time.Time) (*tls.Certificate, error) {
	return loadPair("listen.tls", certFile, keyFile, now)
}

// LoadIssuer reads the issuer the proxy signs its certificates for vendor
// hosts with, as LoadCertificate does, and checks that it is an issuer:
// a certificate authority allowed to sign certificates.
func LoadIssuer(certFile, keyFile string, now time.Time) (*tls.Certificate, error) {
	cert, err := loadPair("listen.connect", certFile, keyFile, now)
	if err != nil {
		return nil, err
	}
	if leaf := cert.Leaf; !leaf.IsCA || leaf.KeyUsage != 0 && leaf.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, fmt.Errorf("%w: listen.connect: the certificate in %s is not an issuer: it may not sign certificates", ErrInvalid, certFile)
	}
	return cert, nil
}

func loadPair(label, certFile, keyFile string, now time.Time) (*tls.Certificate, error) {
	certPEM, certErr := os.ReadFile(certFile) //nolint:gosec // the config names its certificate files
	keyPEM, keyErr := os.ReadFile(keyFile)    //nolint:gosec // the config names its certificate files
	if err := errors.Join(certErr, keyErr); err != nil {
		return nil, fmt.Errorf("%w: %s: the cert or key file cannot be read: %w", ErrInvalid, label, err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %s and %s are not a matching certificate and key: %w", ErrInvalid, label, certFile, keyFile, err)
	}
	if notAfter := cert.Leaf.NotAfter; now.After(notAfter) {
		return nil, fmt.Errorf("%w: %s: the certificate in %s expired on %s", ErrInvalid, label, certFile, notAfter.UTC().Format(time.RFC3339))
	}
	return &cert, nil
}

func (l *loader) path(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.dir, path)
	}
	return path
}

func (l *loader) spec(label, path string) *spec.Spec {
	if path == "" {
		return nil
	}
	path = l.path(path)
	if s, read := l.specs[path]; read {
		return s
	}
	data, err := os.ReadFile(path) //nolint:gosec // the config names its spec files
	if err != nil {
		l.problem("%s: spec %s cannot be read: %v", label, path, err)
		return nil
	}
	s, err := spec.Parse(path, data)
	if err != nil {
		l.problems = append(l.problems, err)
	}
	l.specs[path] = s
	return s
}
