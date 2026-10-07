VERSION := $(shell node -p "require('./package.json').version")

# The .vsix files are per platform, so an install needs the host target.
HOST_OS := $(shell uname -s)
HOST_ARCH := $(shell uname -m)
ifeq ($(HOST_OS),Darwin)
  HOST_TARGET := $(if $(filter arm64,$(HOST_ARCH)),darwin-arm64,darwin-x64)
else
  HOST_TARGET := $(if $(filter aarch64 arm64,$(HOST_ARCH)),linux-arm64,linux-x64)
endif
VSIX := tilt-tools-$(VERSION)-$(HOST_TARGET).vsix
CODE ?= code

.PHONY: all
all: build

node_modules: package.json
	npm install
	@touch node_modules

.PHONY: build
build: node_modules
	npm run compile

.PHONY: typecheck
typecheck: node_modules
	npm run typecheck

.PHONY: test
test: node_modules
	npm test

.PHONY: check
check: typecheck test lsp-test

# The package script cross-compiles the language server per target, so this
# does not depend on the lsp target.
.PHONY: package
package: node_modules check
	npm run package
	node scripts/package.js build

# One .vsix for this machine only, which is all an install needs.
.PHONY: package-host
package-host: node_modules
	npm run package
	VSCE_TARGET=$(HOST_TARGET) node scripts/package.js build

.PHONY: install
install: package-host
	$(CODE) --install-extension $(VSIX) --force
	@echo "Installed $(VSIX). Reload VS Code to pick it up."

.PHONY: uninstall
uninstall:
	$(CODE) --uninstall-extension owenrumney.tilt-tools

.PHONY: clean
clean:
	rm -rf dist out bin lsp/bin *.vsix

# --- Tiltfile language server (lsp/) ---

LSP_BINARY := bin/tiltfile-lsp

.PHONY: lsp
lsp:
	cd lsp && go build -ldflags "-X main.version=$(VERSION)-dev"  -o ../$(LSP_BINARY) ./cmd/tiltfile-lsp

.PHONY: lsp-test
lsp-test:
	cd lsp && go test ./...

.PHONY: lsp-fmt
lsp-fmt:
	cd lsp && gofmt -l -w .

# Refresh the vendored Tiltfile API stubs from the installed Tilt.
.PHONY: lsp-dump
lsp-dump:
	tilt dump api-docs --dir lsp/internal/builtins/gen

# Regenerate the builtin table from the vendored stubs.
.PHONY: lsp-gen
lsp-gen:
	cd lsp && go run ./internal/builtins/gen \
		-in internal/builtins/gen/api \
		-out internal/builtins/builtins_gen.go \
		-grammar ../resources/tiltfile.tmLanguage.json
	cd lsp && gofmt -w internal/builtins/builtins_gen.go

# Download real Tiltfiles from public repositories into .corpus/.
.PHONY: lsp-corpus-fetch
lsp-corpus-fetch:
	node scripts/fetch-corpus.js

# Run the corpus tests. TILT_CORPUS defaults to what lsp-corpus-fetch writes.
TILT_CORPUS ?= $(CURDIR)/.corpus

.PHONY: lsp-corpus
lsp-corpus:
	cd lsp && TILT_CORPUS=$(TILT_CORPUS) go test ./internal/corpus/ -v

# Validate .goreleaser.yml, and build the release artifacts locally without
# touching GitHub. Writes to dist-release/.
.PHONY: lsp-release-check
lsp-release-check:
	goreleaser check

.PHONY: lsp-release-snapshot
lsp-release-snapshot:
	goreleaser release --snapshot --clean --skip=publish
