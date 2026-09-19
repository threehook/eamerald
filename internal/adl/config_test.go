//nolint:testpackage // outputs()/ConfigFromPlugins internals are unexported and only need to be verified from within the package.
package adl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestConfigFromPlugins_AbsentKeyIsDisabled(t *testing.T) {
	cfg, err := ConfigFromPlugins(nil)
	require.NoError(t, err)

	assert.False(t, cfg.Enabled)
	assert.Equal(t, defaultOutput, cfg.Output)
}

func TestConfigFromPlugins_ReadsFullConfig(t *testing.T) {
	cfg, err := ConfigFromPlugins(map[string]any{
		ConfigKey: map[string]any{
			"enabled":  true,
			"output":   "stdout",
			"otlp":     map[string]any{"endpoint": testOTLPEndpoint, "insecure": true},
			"resource": map[string]any{testServiceKey: testServiceName},
			"resource_context": map[string]any{
				"type_key": testTypeKey,
				"id_key":   testIDKey,
			},
		},
	})
	require.NoError(t, err)

	assert.True(t, cfg.Enabled)
	assert.Equal(t, OutputStdout, cfg.Output)
	assert.Equal(t, testOTLPEndpoint, cfg.OTLP.Endpoint)
	assert.True(t, cfg.OTLP.Insecure)
	assert.Equal(t, map[string]string{testServiceKey: testServiceName}, cfg.Resource)
	assert.Equal(t, testTypeKey, cfg.ResourceContext.typeKey())
	assert.Equal(t, testIDKey, cfg.ResourceContext.idKey())
}

func TestConfigFromPlugins_UnsetResourceContextKeysFallBackToTopazConvention(t *testing.T) {
	cfg, err := ConfigFromPlugins(map[string]any{
		ConfigKey: map[string]any{"enabled": true},
	})
	require.NoError(t, err)

	assert.Equal(t, defaultResourceTypeKey, cfg.ResourceContext.typeKey())
	assert.Equal(t, defaultResourceIDKey, cfg.ResourceContext.idKey())
}

func TestConfigFromPlugins_RejectsMistypedConfig(t *testing.T) {
	_, err := ConfigFromPlugins(map[string]any{ConfigKey: "not a config object"})

	require.Error(t, err)
}

// Running one PDP per policy is only useful if their records can be told
// apart, so every record carries an instance identifier whether or not the
// deployment configured a resource.
func TestConfig_EffectiveResource_AlwaysIdentifiesTheInstance(t *testing.T) {
	resource := Config{}.effectiveResource()

	assert.NotEmpty(t, resource[resourceKeyInstanceID])
}

func TestConfig_EffectiveResource_KeepsConfiguredIdentity(t *testing.T) {
	resource := Config{Resource: map[string]string{"service.name": "eamerald"}}.effectiveResource()

	assert.Equal(t, "eamerald", resource["service.name"])
	assert.NotEmpty(t, resource[resourceKeyInstanceID])
}

func TestConfig_EffectiveResource_DoesNotOverrideAnExplicitInstanceID(t *testing.T) {
	resource := Config{Resource: map[string]string{resourceKeyInstanceID: "pdp-laadpalen-0"}}.effectiveResource()

	assert.Equal(t, "pdp-laadpalen-0", resource[resourceKeyInstanceID])
}
