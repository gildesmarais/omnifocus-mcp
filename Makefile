.DEFAULT_GOAL := check

BIN := omnifocus-mcp
MISE := $(shell command -v mise >/dev/null 2>&1 && printf 'mise exec -- ')
GO := $(MISE)go
GOFMT := $(MISE)gofmt

.PHONY: check lint lintfix ready build live

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
