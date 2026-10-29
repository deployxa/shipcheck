# Shipcheck — single makefile for build, check, docs, fixtures, release.

GO ?= go
VERSION ?= 0.1.0-rc1
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/deployxa/shipcheck/internal/cli.Version=$(VERSION) -X github.com/deployxa/shipcheck/internal/cli.Commit=$(COMMIT)

SHA256 ?= $(shell command -v sha256sum 2>/dev/null || echo "shasum -a 256")
BIN := bin/shipcheck
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
DIST := dist

.PHONY: build test check vet fmt docs fixtures release clean

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/shipcheck

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	@out=$$($(GO) fmt ./...); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

# Full local gate: format, vet, tests.
check: fmt vet test

# Regenerate the rule catalogue documentation from the registry.
docs: build
	$(BIN) rules --format markdown > docs/rules.md

# Recreate the synthetic test fixtures, including the escaping symlinks.
fixtures:
	bash scripts/create_fixtures.sh
	ln -sfn /etc/hostname testdata/fixtures/security-traps/leak.txt
	ln -sfn /etc testdata/fixtures/security-traps/leak-dir

# Cross-platform release artifacts with SHA256 checksums.
release: clean
	@mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%%/*}; arch=$${p##*/}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		name=shipcheck-$(VERSION)-$$os-$$arch; \
		echo "building $$name"; \
		GOOS=$$os GOARCH=$$arch $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/$$name$$ext ./cmd/shipcheck; \
		if [ "$$os" = "windows" ]; then \
			(cd $(DIST) && zip -q $$name.zip $$name$$ext && rm $$name$$ext); \
		else \
			tar -czf $(DIST)/$$name.tar.gz -C $(DIST) $$name && rm $(DIST)/$$name; \
		fi; \
	done
	cd $(DIST) && $(SHA256) * > shipcheck-$(VERSION)-checksums.sha256

clean:
	rm -rf $(DIST) bin
