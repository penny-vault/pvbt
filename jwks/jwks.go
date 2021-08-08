package jwks

import (
	"context"
	"fmt"

	"github.com/lestrrat-go/jwx/jwk"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// LoadJWKS retrieves JWKS from auth0 domain
func SetupJWKS() (*jwk.AutoRefresh, string) {
	// read remote JWKS
	jwksUrl := fmt.Sprintf("https://%s/.well-known/jwks.json", viper.GetString("auth0.domain"))

	log.WithFields(log.Fields{
		"Url": jwksUrl,
	}).Debug("reading JWKS")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ar := jwk.NewAutoRefresh(ctx)
	ar.Configure(jwksUrl)
	ar.Fetch(ctx, jwksUrl) // perform initial fetch

	return ar, jwksUrl
}
