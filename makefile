.PHONY: help \
	build format test \
	clean

GOCMD = go
GOTEST = $(GOCMD) test
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean

BINARY = totally
DIST_DIR = dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.DEFAULT_GOAL := help

##@ Help
## help: Display this informational message
help:
	@awk ' \
		function flush() { \
			if (target == "") return; \
			line = sprintf("  %-16s %s", target, desc); \
			if (options != "") line = line " [" options "]"; \
			printf "%s\n", line; \
			target = ""; \
			desc = ""; \
			options = ""; \
		} \
		/^##@/ {next} \
		/^##   options:/ {pending_options = substr($$0, 15); next} \
		/^## / { \
			line = substr($$0, 4); \
			split(line, parts, ": "); \
			pending_desc = substr(line, length(parts[1]) + 3); \
			next; \
		} \
		/^[a-zA-Z0-9_.-]+:[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=/ {next} \
		/^\.(PHONY|DEFAULT_GOAL)/ {next} \
		/^[a-zA-Z0-9_.-]+:/ { \
			flush(); \
			split($$1, parts, ":"); \
			target = parts[1]; \
			desc = pending_desc; \
			options = pending_options; \
			pending_desc = ""; \
			pending_options = ""; \
			next; \
		} \
		END {flush()} \
	' $(MAKEFILE_LIST)

##@ Build
## build: Build a compressed release bundle
##   options: VERSION=..., OS=..., ARCH=...
build:
	@echo "Bundling $(BINARY) $(VERSION) for $(OS)/$(ARCH)..."
	@mkdir -p "$(DIST_DIR)"
	GOOS="$(OS)" GOARCH="$(ARCH)" $(GOBUILD) -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST_DIR)/$(BINARY)" ./cmd/totally
	@tar -C "$(DIST_DIR)" -czf "$(DIST_DIR)/$(BINARY)-$(VERSION)-$(OS)-$(ARCH).tar.gz" "$(BINARY)"
	@echo "Bundle written to $(DIST_DIR)/$(BINARY)-$(VERSION)-$(OS)-$(ARCH).tar.gz"

##@ Code Quality
## format: Format Go source
format:
	@echo "Formatting Go files with gofmt..."
	@find . -name '*.go' \
		-not -path './vendor/*' \
		-not -path './.cache/*' \
		-not -path './$(DIST_DIR)/*' \
		-print0 | xargs -0 gofmt -w
	@if command -v goimports >/dev/null; then \
		echo "Formatting imports with goimports..."; \
		find . -name '*.go' \
			-not -path './vendor/*' \
			-not -path './.cache/*' \
			-not -path './$(DIST_DIR)/*' \
			-print0 | xargs -0 goimports -w; \
	else \
		echo "goimports not found; gofmt completed"; \
	fi

##@Lint
## lint: Lint Go source
lint:
	@echo "Vetting Go files with go vet..."
	@go vet ./...

##@ Test
## test: Run all Go tests
test:
	$(GOTEST) -v ./...

##@ Maintenance
## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf "$(DIST_DIR)"
	@echo "Clean complete"
