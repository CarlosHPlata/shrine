.PHONY: build test test-integration test-all docs-serve docs-build docs-gen-cli docs-tools docs-check

# Hugo binary: prefer one already on PATH; otherwise fall back to where
# `go install` drops it. `make docs-tools` installs the pinned version.
HUGO_VERSION := v0.161.1
HUGO ?= $(shell command -v hugo 2>/dev/null || echo $(shell go env GOPATH)/bin/hugo)

build:
	go build -o shrine .

test:
	go test ./...

test-integration:
	go test -tags integration -v -timeout 5m ./tests/integration/...

test-all: test test-integration

docs-tools:
	CGO_ENABLED=1 go install -tags extended github.com/gohugoio/hugo@$(HUGO_VERSION)

docs-serve:
	cd docs && $(HUGO) serve --buildDrafts --navigateToChanged --bind 0.0.0.0

docs-build:
	cd docs && $(HUGO) --gc --minify

docs-gen-cli:
	cd docs/tools/docsgen && go run ./cmd/docsgen -out ../../content/cli -clean

docs-check:
	bash scripts/lint-docs-frontmatter.sh docs/content
	cd docs/tools/docsgen && go test ./...
	@echo "Drift check (CLI reference vs Cobra tree):"
	@mkdir -p /tmp/expected-cli && cd docs/tools/docsgen && go run ./cmd/docsgen -out /tmp/expected-cli && \
	  diff -r /tmp/expected-cli ../../content/cli --exclude=_index.md --exclude=.gitkeep && \
	  echo "OK: CLI reference is in sync"
	$(MAKE) docs-build
	bash scripts/check-md-companions.sh docs/public
	bash scripts/check-md-shape.sh docs/public
