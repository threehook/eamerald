package fflag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threehook/eamerald/topaz/fflag"
)

func TestFFlag(t *testing.T) {
	{
		ff := fflag.FFlag(0)
		assert.False(t, ff.IsSet(fflag.Editor))
	}
	{
		ff := fflag.FFlag(1)
		assert.True(t, ff.IsSet(fflag.Editor))
	}
	{
		ff := fflag.FFlag(2)
		assert.False(t, ff.IsSet(fflag.Editor))
	}
	{
		ff := fflag.FFlag(3)
		assert.True(t, ff.IsSet(fflag.Editor))
	}

	{
		ff := fflag.FFlag(31)
		assert.True(t, ff.IsSet(fflag.Editor))
	}
	{
		ff := fflag.FFlag(63)
		assert.True(t, ff.IsSet(fflag.Editor))
	}
}
