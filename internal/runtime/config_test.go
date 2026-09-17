package runtime_test

import (
	"testing"

	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/require"
	runtime "github.com/threehook/eamerald/internal/runtime"
)

func TestDeepCopy(t *testing.T) {
	assert := require.New(t)
	runtimeConfig := &runtime.Config{}

	_, err := copystructure.Copy(runtimeConfig)

	assert.NoError(err)
}
