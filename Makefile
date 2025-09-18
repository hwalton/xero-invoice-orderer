SHELL := /bin/bash

.PHONY: install-air build-css watch-css dev

install-air:
	@echo "Installing air..."
	go install github.com/cosmtrek/air@latest
	@echo "air installed to $(shell go env GOPATH)/bin"

# Build production CSS (minified)
build-css:
	npx tailwindcss -i ./src/internal/web/static/tailwind/input.css -o ./src/internal/web/static/tailwind/tailwind.css --minify

# Watch CSS for development (blocking)
watch-css:
	npx tailwindcss -i ./src/internal/web/static/tailwind/input.css -o ./src/internal/web/static/tailwind/tailwind.css --watch

