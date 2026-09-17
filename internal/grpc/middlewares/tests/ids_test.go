package tests_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threehook/eamerald/internal/header"
)

func RequestIDContext(t *testing.T) context.Context {
	assert := require.New(t)

	id, err := uuid.NewUUID()
	assert.NoError(err)

	return context.WithValue(context.Background(), header.HeaderAsertoRequestID, id)
}
