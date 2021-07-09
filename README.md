# pv-api

[![Build Status](https://travis-ci.com/jdfergason/pv-api.svg?branch=master)](https://travis-ci.com/jdfergason/pv-api)
[![Go Report Card](https://goreportcard.com/badge/github.com/jdfergason/pv-api)](https://goreportcard.com/report/github.com/jdfergason/pv-api)
[![codecov](https://codecov.io/gh/jdfergason/pv-api/branch/master/graph/badge.svg?token=L3C272LW9C)](https://codecov.io/gh/jdfergason/pv-api)

Penny Vault HTTPS api that is deployed to Heroku.

# List of environment variable configuration

- AUTH0_CLIENT_ID: configuration for auth0
- AUTH0_DOMAIN: configuration for auth0
- AUTH0_SECRET: configuration for auth0
- DATABASE_URL: database connection string
- LOKI_URL: URL to send loki logs to
- PORT: port server should listen on
- PV_SECRET: hex-encoded 32-byte key used for encrypting / decrypting API keys
- SENDGRID_API_KEY: sendgrid api key
- SYSTEM_TIINGO_TOKEN: tiingo token to use when computing overall strategy performance
