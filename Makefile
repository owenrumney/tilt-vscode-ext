VSIX := tilt-viewer-$(shell node -p "require('./package.json').version").vsix
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
check: typecheck test

.PHONY: package
package: node_modules check
	npm run package
	npx vsce package --no-dependencies --out $(VSIX)

.PHONY: install
install: package
	$(CODE) --install-extension $(VSIX) --force
	@echo "Installed $(VSIX). Reload VS Code to pick it up."

.PHONY: uninstall
uninstall:
	$(CODE) --uninstall-extension owenrumney.tilt-viewer

.PHONY: clean
clean:
	rm -rf dist out *.vsix
