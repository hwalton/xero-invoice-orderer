SHELL := /bin/bash

# Project-wide vars
TAILWIND = npx @tailwindcss/cli
CONFIG    = ./tailwind.config.js
INPUT     = ./src/internal/frontend/static/tailwind/input.css
OUTPUT    = ./src/internal/frontend/static/tailwind/output.css

.PHONY: install install-air watch-css build-css dev

install:
	npm install

install-air:
	go install github.com/cosmtrek/air/v2/cmd/air@latest
	@echo "add $(shell go env GOPATH)/bin to your PATH if needed"

watch-css:
	$(TAILWIND) -c $(CONFIG) -i $(INPUT) -o $(OUTPUT) --watch

build-css:
	$(TAILWIND) -c $(CONFIG) -i $(INPUT) -o $(OUTPUT) --minify

dev:
	# start tailwind watcher in background, then run air from src (uses src/.air.toml)
	$(MAKE) watch-css & \
	sleep 0.2; \
	cd src && air