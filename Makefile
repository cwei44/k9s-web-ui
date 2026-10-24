GO ?= go

.PHONY: build test package clean
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags '-s -w' -o k9s-web-ui .

test:
	$(GO) test ./...
	$(GO) vet ./...
	npm ci --include=dev --ignore-scripts
	npm test

package: build
	python3 scripts/package.py

clean:
	rm -rf dist k9s-web-ui
