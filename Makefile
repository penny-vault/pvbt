# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build -v
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test

all: test pvapi notifier

pvapi:
	$(GOBUILD) -o bin/pvapi -v cmd/pvapi/main.go

notifier:
	$(GOBUILD) -o bin/notifier -v cmd/notifier/main.go

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f bin/pvapi
	rm -f bin/notifier
