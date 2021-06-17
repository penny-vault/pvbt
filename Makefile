# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build -v
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test

all: test pvapi notifier

pvapi:
	$(GOBUILD) -o bin/pvapi -tags jwx_goccy -v cmd/pvapi/main.go

notifier:
	$(GOBUILD) -o bin/notifier -tags jwx_goccy -v cmd/notifier/main.go cmd/notifier/auth0.go

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f bin/pvapi
	rm -f bin/notifier
