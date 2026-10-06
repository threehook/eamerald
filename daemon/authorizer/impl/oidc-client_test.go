//go:build integration

//nolint:testpackage
package impl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	googleIssuer = "https://accounts.google.com"
	auth0Issuer  = "https://aserto.us.auth0.com/"
)

func TestOidcClient(t *testing.T) {
	ctx := t.Context()

	issuers := []string{auth0Issuer, googleIssuer}

	client := NewOidcClient()

	for _, issuer := range issuers {
		config, err := client.FetchAndValidateConfig(ctx, issuer)
		require.NoError(t, err)

		t.Logf("Validated Issuer: %s\n", config.Issuer)
		t.Logf("Validated JWKS URI: %s\n", config.JwksURI)
	}
}
