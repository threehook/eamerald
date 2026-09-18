//nolint:testpackage // outputs() is unexported and only needs to be verified from within the package.
package adl_decision_logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Outputs_DefaultsToBothWhenUnset(t *testing.T) {
	cfg := Config{}

	outputs := cfg.outputs()

	assert.True(t, outputs[OutputStdout])
	assert.True(t, outputs[OutputOTLP])
	assert.Len(t, outputs, 2)
}

func TestConfig_Outputs_RespectsExplicitSingleValue(t *testing.T) {
	cfg := Config{Output: OutputOTLP}

	outputs := cfg.outputs()

	assert.False(t, outputs[OutputStdout])
	assert.True(t, outputs[OutputOTLP])
	assert.Len(t, outputs, 1)
}

func TestConfig_Outputs_ParsesCommaSeparatedWithWhitespace(t *testing.T) {
	cfg := Config{Output: " stdout , otlp "}

	outputs := cfg.outputs()

	assert.True(t, outputs[OutputStdout])
	assert.True(t, outputs[OutputOTLP])
	assert.Len(t, outputs, 2)
}

func TestConfig_Outputs_IgnoresUnknownValues(t *testing.T) {
	// outputs() itself does no validation - an operator typo just yields a
	// set the caller won't recognize, rather than panicking or defaulting.
	cfg := Config{Output: "stdout,carrier-pigeon"}

	outputs := cfg.outputs()

	assert.True(t, outputs[OutputStdout])
	assert.False(t, outputs[OutputOTLP])
	assert.True(t, outputs["carrier-pigeon"])
}
