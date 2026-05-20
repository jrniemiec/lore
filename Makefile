# Makefile for lore (multi-package layout, main package in repo root)

APP        := lore
LINK_NAME  := $(APP)
PKG        := ./...
BIN_DIR    := bin
OUT        := $(BIN_DIR)/$(APP)

GO         ?= go
GOFLAGS    ?=
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    ?= -X main.version=$(VERSION)
TAGS       ?=

# Helpful defaults
SHELL      := /bin/bash
.DEFAULT_GOAL := build

# --- Installation setup ---
BIN_NAME ?= lore
GOBIN := $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell $(GO) env GOPATH)/bin
endif
DEV_BIN  ?= $(HOME)/dev/bin

LORE_HOME     ?= $(HOME)/.lore
LORE_CONFIG   ?= $(LORE_HOME)/config.json

# --- Helpers ---
.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

.PHONY: info
info: ## Print Go/tooling info
	@echo "APP:        $(APP)"
	@echo "OUT:        $(OUT)"
	@echo "LORE_HOME:  $(LORE_HOME)"
	@$(GO) version
	@$(GO) env GOPATH GOMOD GOCACHE GOOS GOARCH

# --- Build / Run ---
.PHONY: build
build: ## Build binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $(OUT) .
	ln -sf $(BIN_DIR)/$(APP) ./$(LINK_NAME)

.PHONY: build-shared
build-shared: ## Build binary using shared llm module (github.com/jrniemiec/llm)
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -tags 'sharedllm' -ldflags '$(LDFLAGS)' -o $(OUT) .
	ln -sf $(BIN_DIR)/$(APP) ./$(LINK_NAME)

.PHONY: install-shared
install-shared: test build-shared ## Install using shared llm module
	@mkdir -p "$(DEV_BIN)"
	@rm -f "$(DEV_BIN)/$(BIN_NAME)"
	@cp "$(OUT)" "$(DEV_BIN)/$(BIN_NAME)"
	@chmod +x "$(DEV_BIN)/$(BIN_NAME)"
	@echo "Installed: $(DEV_BIN)/$(BIN_NAME)"

.PHONY: run
run: ## Run the app (pass args via ARGS="...")
	$(GO) run $(GOFLAGS) . $(ARGS)

.PHONY: install
install: test build ## Install into ~/dev/bin
	@mkdir -p "$(DEV_BIN)"
	@rm -f "$(DEV_BIN)/$(BIN_NAME)"
	@cp "$(OUT)" "$(DEV_BIN)/$(BIN_NAME)"
	@chmod +x "$(DEV_BIN)/$(BIN_NAME)"
	@echo "Installed: $(DEV_BIN)/$(BIN_NAME)"

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf $(BIN_DIR)
	@rm -f $(LINK_NAME)
	@rm *~ .*~ 2>/dev/null || true

# --- Code Quality ---
.PHONY: fmt
fmt: ## gofmt all Go files
	@$(GO) fmt $(PKG)

.PHONY: fmt-check
fmt-check: ## Fail if files are not gofmt'd
	@unformatted=$$($(GO) fmt $(PKG) 2>/dev/null); \
	if [[ -n "$$unformatted" ]]; then \
		echo "These files were not formatted (gofmt applied):"; \
		echo "$$unformatted"; \
		echo "Run: make fmt"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: check
check: fmt-check vet lint test ## Run all checks (fmt, vet, lint, test)

# --- Tests ---
.PHONY: test
test: ## Run unit tests
	$(GO) test $(GOFLAGS) $(PKG)

.PHONY: testv
testv: ## Run unit tests (verbose)
	$(GO) test $(GOFLAGS) -v $(PKG)

.PHONY: race
race: ## Run tests with the race detector
	$(GO) test $(GOFLAGS) -race $(PKG)

.PHONY: bench
bench: ## Run benchmarks
	$(GO) test $(GOFLAGS) -bench=. -benchmem $(PKG)

# --- Optional lint (golangci-lint) ---
.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

# --- Dev convenience ---
.PHONY: dev
dev: build ## Build + print a quick usage hint
	@echo
	@echo "Built: $(OUT)"
	@echo "Try:   $(OUT) --help"

.PHONY: repo-commit
repo-commit: ## git-commit called for conditional commit
	@git-commit

.PHONY: repo-push
repo-push: ## then push to origin
	@git-push

.PHONY: release
release: ## Tag and push a release (usage: make release VERSION=0.9.3)
	@if [ -z "$(VERSION)" ]; then echo "usage: make release VERSION=x.y.z"; exit 1; fi
	git tag v$(VERSION)
	git push origin v$(VERSION)
