.DEFAULT_GOAL := check

BIN := omnifocus-mcp
MISE := $(shell command -v mise >/dev/null 2>&1 && printf 'mise exec -- ')
GO := $(MISE)go
GOFMT := $(MISE)gofmt
GREMLINS := $(MISE)gremlins
PKG ?= .
DIFF ?=
TIMEOUT_COEF ?= 30

.PHONY: check lint lintfix ready build live mutate

check:
	$(GO) test ./...

lint:
	@files=$$($(GOFMT) -l .); \
	if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi
	$(GO) vet ./...

lintfix:
	$(GOFMT) -w .

ready: lint check build

build:
	$(GO) build -o $(BIN) .

# Opt-in: runs against the real OmniFocus (needs it running and Automation
# permission). Reads your database and creates, edits and deletes its own
# objects in an "MCP Sandbox" project, "MCP Sandbox Folder" and "mcp-test-*" tags.
live:
	$(GO) test -tags live -count=1 -v ./internal/omni/...

# Opt-in mutation testing (unit tests only, no live tag). PKG narrows the path
# (default: the module root, so repo-relative DIFF paths match);
# DIFF=<ref> mutates only code changed since that ref.
mutate:
	$(GREMLINS) unleash $(if $(DIFF),--diff $(DIFF)) --timeout-coefficient $(TIMEOUT_COEF) $(PKG)
