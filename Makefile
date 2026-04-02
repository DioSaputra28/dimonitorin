APP_NAME := dimonitorin
MAIN_PKG := ./cmd/dimonitorin
DIST_DIR := dist
BUILD_DIR := $(DIST_DIR)/build
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
GOFLAGS := -trimpath
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
GO_FILES := $(shell if command -v rg >/dev/null 2>&1; then rg --files -g'*.go'; else find . -type f -name '*.go' -not -path './node_modules/*'; fi)

.PHONY: deps assets generate fmt build test clean package release install-script help

help:
	@echo "Targets: deps assets generate fmt build test package release clean"

deps:
	go mod tidy
	npm install

assets:
	npx tailwindcss -i ./static/src/input.css -o ./static/css/app.css --minify
	cp node_modules/htmx.org/dist/htmx.min.js static/js/htmx.min.js
	cp node_modules/echarts/dist/echarts.min.js static/js/echarts.min.js

generate:
	/root/go/bin/templ generate ./internal/views

fmt:
	gofmt -w $(GO_FILES)

build: generate assets fmt
	mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

test: generate assets fmt
	go test ./...

package: generate assets fmt
	./scripts/package-release.sh "$(VERSION)"

release: clean deps package

install-script:
	chmod +x scripts/install.sh scripts/package-release.sh

clean:
	rm -rf $(DIST_DIR)
