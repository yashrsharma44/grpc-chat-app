# include .env #if there is a .env file uncomment this line

PROJECTNAME=$(shell basename "$(PWD)")

GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

SERVER_FILES = ./grpc-chatapp/server/
CLIENT_FILES = ./grpc-chatapp/client/
GOBIN=./bin

# Redirect error output to a file, so we can show it in development mode.
STDERR=/tmp/.$(PROJECTNAME)-stderr.txt

go-clean:
	@echo "  >  Cleaning build cache"
	@GOBIN=$(GOBIN) go clean

help: Makefile
	@echo "\nChoose a command run in "$(PROJECTNAME)":\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'

go-compile-server: go-build-server

go-build-server:
	@echo "  >  Building server binary..."
	@GOBIN=$(GOBIN) go build -o $(GOBIN)/server $(SERVER_FILES)

## build-server: Building the Server binary.
build-server:
	@-touch $(STDERR)
	@-rm $(STDERR)
	@-$(MAKE) -s go-compile-server 2> $(STDERR)
	@cat $(STDERR) | sed -e '1s/.*/\nError:\n/'  | sed 's/make\[.*/ /' | sed "/^/s/^/     /" 1>&2

## run-server: Build and run the Server binary
run-server: build-server
	@echo "  >  Running server binary..."
	@-$(GOBIN)/server

go-compile-client: go-build-client

go-build-client:
	@echo "  >  Building client binary..."
	@GOBIN=$(GOBIN) go build -o $(GOBIN)/client $(CLIENT_FILES)

## build-client: Building the Client binary.
build-client:
	@-touch $(STDERR)
	@-rm $(STDERR)
	@-$(MAKE) -s go-compile-client 2> $(STDERR)
	@cat $(STDERR) | sed -e '1s/.*/\nError:\n/'  | sed 's/make\[.*/ /' | sed "/^/s/^/     /" 1>&2

## run-client: Build and run the Client binary
run-client: build-client
	@echo "  >  Running client binary..."
	@-$(GOBIN)/client

## exec: Run given command, wrapped with custom GOPATH. e.g; make exec run="go test ./..."
exec:
	@GOBIN=$(GOBIN) $(run)

## clean: Clean build files. Runs `go clean` internally.
clean: go-clean
