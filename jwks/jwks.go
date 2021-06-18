package jwks

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/goccy/go-json"

	"github.com/lestrrat-go/jwx/jwk"
	log "github.com/sirupsen/logrus"
)

// AuthConfig stores configuration related to JWKS
type authConfig struct {
	Domain   string `json:"domain"`
	Audience string `json:"audience"`
}

// LoadJWKS load settings from auth.json and retrieve JWKS
func SetupJWKS() (*jwk.AutoRefresh, string) {
	// Load settings
	jsonFile, err := os.Open("auth.json")
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Successfully opened auth.json")
	// defer the closing of our jsonFile so that we can parse it later on
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	var config authConfig
	json.Unmarshal(byteValue, &config)

	// read remote JWKS
	jwksUrl := fmt.Sprintf("https://%s/.well-known/jwks.json", config.Domain)
	log.Printf("Reading JWKS from %s\n", jwksUrl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ar := jwk.NewAutoRefresh(ctx)
	ar.Configure(jwksUrl)
	ar.Fetch(ctx, jwksUrl) // perform initial fetch

	return ar, jwksUrl
}
