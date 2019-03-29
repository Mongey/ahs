NAME          := ahs
VERSION       := $(shell git describe --tags --abbrev=1)
FILES         := $(shell git ls-files '*.go')
LDFLAGS       := -w -extldflags "-static" -X 'main.version=$(VERSION)'
REGISTRY      := mvisonneau/$(NAME)
.DEFAULT_GOAL := help

export GO111MODULE=on

.PHONY: setup
setup: ## Install required libraries/tools
	GO111MODULE=off go get -u -v github.com/mitchellh/gox
	GO111MODULE=off go get -u -v github.com/tcnksm/ghr
	GO111MODULE=off go get -u -v golang.org/x/lint/golint
	GO111MODULE=off go get -u -v golang.org/x/tools/cmd/cover
	GO111MODULE=off go get -u -v golang.org/x/tools/cmd/goimports

.PHONY: fmt
fmt: ## Format source code
	goimports -w $(FILES)

.PHONY: lint
lint: ## Run golint and go vet against the codebase
	golint -set_exit_status .
	go vet ./...

.PHONY: test
test: ## Run the tests against the codebase
	go test -v ./...

.PHONY: install
install: ## Build and install locally the binary (dev purpose)
	go install .

.PHONY: build
build: ## Build the binary
	mkdir -p dist; rm -rf dist/*
	CGO_ENABLED=0 gox -osarch "linux/386 linux/amd64 linux/arm64" -ldflags "$(LDFLAGS)" -output dist/$(NAME)_{{.OS}}_{{.Arch}}
	strip dist/*_linux_amd64 dist/*_linux_386

.PHONY: build-docker
build-docker:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" .
	strip $(NAME)

.PHONY: publish-github
publish-github: ## Send the binaries onto the GitHub release
	ghr -u mvisonneau -replace $(VERSION) dist

.PHONY: imports
imports: ## Fixes the syntax (linting) of the codebase
	goimports -d $(FILES)

.PHONY: clean
clean: ## Remove binary if it exists
	rm -f $(NAME)

.PHONY: coverage
coverage: ## Generates coverage report
	rm -rf *.out
	go test -coverprofile=coverage.out

.PHONY: dev-env
dev-env: ## Build a local development environment using Docker
	@docker run -it --rm \
		-v $(shell pwd):/go/src/github.com/mvisonneau/$(NAME) \
		-w /go/src/github.com/mvisonneau/$(NAME) \
		golang:1.12 \
		/bin/bash -c 'make setup; make install; bash'

.PHONY: all
all: lint imports test coverage build ## Test, builds and ship package for all supported platforms

.PHONY: help
help: ## Displays this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
