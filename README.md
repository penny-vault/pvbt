# pv-api

[![Build](https://github.com/penny-vault/pv-api/actions/workflows/build.yml/badge.svg)](https://github.com/penny-vault/pv-api/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jdfergason/pv-api)](https://goreportcard.com/report/github.com/penny-vault/pv-api)
[![codecov](https://codecov.io/gh/penny-vault/pv-api/branch/master/graph/badge.svg?token=L3C272LW9C)](https://codecov.io/gh/penny-vault/pv-api)

Penny Vault api provides backend functionality for managing a quantiative invesment portfolio. It has functions for:

- Backtesting a strategy with a user provided set of parameters
- Running a strategy at regular intervals to and recording transactions
- Notifying clients of trade signals
- Calculating portfolio performance

The project is versioned in compliance with [Semantic Versioning 2.0](https://semver.org)

# Building

    mage build

# Running

To run the application use the `pvapi` executable. The HTTPS api can be served with:

    pvapi serve -p 3000

For complete details run:

    pvapi help

# Configuring

There are a number of configuration variables necessary to run pvapi. These can be provided via a toml file (see: config.toml.tmpl for an example), environment variables, or as flags to the command line.

# Design principals

This software follows the design principals laid out in the [12-factor app](https://12factor.net).