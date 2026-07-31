## Payvand — development tasks.
##
## Run `make help` for the list of targets.

GO           ?= go
GOFLAGS      ?=
PKGS         := ./...
COVER_PROFILE := coverage.out

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile every package
	$(GO) build $(GOFLAGS) $(PKGS)

.PHONY: test
test: ## Run the test suite
	$(GO) test $(GOFLAGS) -race $(PKGS)

.PHONY: test-short
test-short: ## Run the test suite without the race detector
	$(GO) test $(GOFLAGS) $(PKGS)

.PHONY: cover
cover: ## Run the tests and write a coverage profile
	$(GO) test $(GOFLAGS) -coverprofile=$(COVER_PROFILE) -covermode=atomic $(PKGS)
	$(GO) tool cover -func=$(COVER_PROFILE) | tail -n 1

.PHONY: cover-html
cover-html: cover ## Open the coverage report in a browser
	$(GO) tool cover -html=$(COVER_PROFILE)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKGS)

.PHONY: fmt
fmt: ## Format the code
	$(GO) fmt $(PKGS)

.PHONY: fmt-check
fmt-check: ## Fail when the code is not formatted
	@unformatted=$$(gofmt -l . | grep -v '^vendor/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not formatted:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: lint
lint: fmt-check vet ## Format check plus go vet

.PHONY: tidy
tidy: ## Tidy the module file (Payvand has no dependencies, so this must stay a no-op)
	$(GO) mod tidy

.PHONY: deps-check
deps-check: ## Fail when a third party dependency sneaks into go.mod
	@if grep -q '^require' go.mod; then \
		echo "payvand must depend on the standard library only, but go.mod has a require block"; exit 1; \
	fi
	@echo "no third party dependencies"

.PHONY: doc
doc: ## Serve the package documentation on http://localhost:6060
	$(GO) run golang.org/x/tools/cmd/godoc@latest -http=:6060

.PHONY: examples
examples: ## Run the offline examples
	$(GO) run ./examples/basic
	$(GO) run ./examples/multigateway

.PHONY: webshop
webshop: ## Start the example shop on http://localhost:8080
	$(GO) run ./examples/webshop

.PHONY: ci
ci: lint deps-check test ## Everything the pipeline runs

.PHONY: clean
clean: ## Remove build and coverage artefacts
	rm -f $(COVER_PROFILE) coverage.html
	$(GO) clean -testcache
