package adl_decision_logger

import "strings"

// Output names accepted in Config.Output (comma-separated).
const (
	OutputStdout = "stdout"
	OutputOTLP   = "otlp"
)

// defaultOutput is used whenever Output is unset (e.g. only `enabled: true`
// was configured) - both outputs on, so a collector outage never silently
// loses the compliance-required stdout trail.
const defaultOutput = OutputStdout + "," + OutputOTLP

type Config struct {
	Enabled bool       `json:"enabled"`
	Output  string     `json:"output"` // comma-separated: "stdout", "otlp", or both. Settable via ${LOG_OUTPUT}.
	OTLP    OTLPConfig `json:"otlp"`
}

type OTLPConfig struct {
	Endpoint string `json:"endpoint"` // e.g. "localhost:4317" (Alloy's OTLP gRPC receiver)
	Insecure bool   `json:"insecure"` // disable TLS - typical for a same-cluster/sidecar collector
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

func defaultConfig() *Config {
	return &Config{
		Enabled: false,
		Output:  defaultOutput,
	}
}
