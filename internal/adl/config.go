// Package adl emits Logius Authorization Decision Log (ADL) 1.0 Level 1
// records for the authorization decisions this PDP evaluates.
//
// Spec: https://gitdocumentatie.logius.nl/publicatie/ftv/adl/1.0.0/
//
// The package deliberately depends only on the AuthZEN wire types and the
// W3C trace context helpers: ADL covers both the authorizer's Is() endpoint
// and the directory's AuthZEN Access API, and the latter runs without an OPA
// runtime, so the logger cannot live inside an OPA plugin.
package adl

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

// ConfigKey is the key the ADL configuration is read from, under the OPA
// plugins config block (opa.config.plugins.adl_decision_logger).
const ConfigKey = "adl_decision_logger"

// Output names accepted in Config.Output (comma-separated).
const (
	OutputStdout = "stdout"
	OutputOTLP   = "otlp"
)

// defaultOutput is used whenever Output is unset (e.g. only `enabled: true`
// was configured) - both outputs on, so a collector outage never silently
// loses the compliance-required stdout trail.
const defaultOutput = OutputStdout + "," + OutputOTLP

// Default keys read from a Topaz resource context to populate the AuthZEN
// resource type/id. See ResourceContextConfig.
const (
	defaultResourceTypeKey = "object_type"
	defaultResourceIDKey   = "object_id"
)

type Config struct {
	Enabled bool       `json:"enabled"`
	Output  string     `json:"output"` // comma-separated: "stdout", "otlp", or both. Settable via ${LOG_OUTPUT}.
	OTLP    OTLPConfig `json:"otlp"`

	// Resource identifies the producer of the log record - the system,
	// application or environment the PDP ran in - and is emitted as the
	// record's `resource` field. The spec leaves the key vocabulary to the
	// implementing organisation, but requires the field to be set whenever
	// records are aggregated outside the producing organisation.
	Resource map[string]string `json:"resource"`

	// ResourceContext names the keys the authorizer reads out of a Topaz
	// resource context to populate the AuthZEN resource type/id.
	ResourceContext ResourceContextConfig `json:"resource_context"`
}

type OTLPConfig struct {
	Endpoint string `json:"endpoint"` // e.g. "localhost:4317" (Alloy's OTLP gRPC receiver)
	Insecure bool   `json:"insecure"` // disable TLS - typical for a same-cluster/sidecar collector
}

// ResourceContextConfig maps a Topaz resource context onto the AuthZEN
// resource type/id. Topaz's resource context is a free-form struct with no
// fixed schema, but AuthZEN requires a resource type, so the keys carrying
// it have to be named. Both default to Topaz's own object_type/object_id
// convention.
type ResourceContextConfig struct {
	TypeKey string `json:"type_key"`
	IDKey   string `json:"id_key"`
}

func (c ResourceContextConfig) typeKey() string {
	if c.TypeKey == "" {
		return defaultResourceTypeKey
	}

	return c.TypeKey
}

func (c ResourceContextConfig) idKey() string {
	if c.IDKey == "" {
		return defaultResourceIDKey
	}

	return c.IDKey
}

// outputs splits Output on commas, trims whitespace, and drops empty
// entries. An unset Output defaults to both stdout and otlp.
func (c Config) outputs() map[string]bool {
	raw := c.Output
	if strings.TrimSpace(raw) == "" {
		raw = defaultOutput
	}

	set := map[string]bool{}

	for o := range strings.SplitSeq(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			set[o] = true
		}
	}

	return set
}

// ConfigFromPlugins reads the ADL configuration out of the OPA plugins
// config block. The logger is built from this once at daemon startup and
// shared by every service that evaluates decisions, rather than being
// instantiated per OPA runtime.
func ConfigFromPlugins(plugins map[string]any) (Config, error) {
	cfg := Config{Output: defaultOutput}

	raw, ok := plugins[ConfigKey]
	if !ok || raw == nil {
		return cfg, nil
	}

	buf, err := json.Marshal(raw)
	if err != nil {
		return cfg, errors.Wrap(err, "error marshaling adl config")
	}

	if err := json.Unmarshal(buf, &cfg); err != nil {
		return cfg, errors.Wrap(err, "error parsing adl config")
	}

	return cfg, nil
}
