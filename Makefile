SHELL := /bin/zsh

PROJECT_ROOT := $(abspath .)
FRONTEND_DIR := $(PROJECT_ROOT)/frontend
GO ?= /Users/penwyp/.local/share/mise/installs/go/$(shell ls /Users/penwyp/.local/share/mise/installs/go | sort -V | tail -n 1)/bin/go
WAILS ?= /Users/penwyp/.local/bin/wails
NPM ?= npm

CLI_ARGS ?=
RUN_ARGS ?=

.PHONY: help dev build package cli desktop test fmt tidy generate doctor frontend-install frontend-build frontend-dev clean

help:
	@echo "TypeLens Make Targets"
	@echo ""
	@echo "  make dev                Run Wails desktop in development mode"
	@echo "  make build              Build all Go packages"
	@echo "  make package            Build desktop application via Wails"
	@echo "  make desktop            Run desktop entry with 'go run .'"
	@echo "  make cli CLI_ARGS='...' Run CLI entry, e.g. make cli CLI_ARGS='dict list'"
	@echo "  make test               Run Go tests and frontend production build"
	@echo "  make fmt                Run gofmt on Go sources"
	@echo "  make tidy               Run go mod tidy"
	@echo "  make generate           Refresh Wails generated bindings"
	@echo "  make doctor             Run wails doctor"
	@echo "  make frontend-install   Install frontend dependencies"
	@echo "  make frontend-build     Build frontend only"
	@echo "  make frontend-dev       Run Vite dev server only"
	@echo "  make clean              Remove frontend dist output"

dev:
	cd $(PROJECT_ROOT) && $(WAILS) dev

build:
	cd $(PROJECT_ROOT) && $(GO) build ./...

package:
	cd $(PROJECT_ROOT) && $(WAILS) build

desktop:
	cd $(PROJECT_ROOT) && $(GO) run . $(RUN_ARGS)

cli:
	cd $(PROJECT_ROOT) && $(GO) run ./cmd/typelens $(CLI_ARGS)

test:
	cd $(PROJECT_ROOT) && $(GO) test ./...
	cd $(FRONTEND_DIR) && $(NPM) run build

fmt:
	cd $(PROJECT_ROOT) && $(GO)fmt -w $$(find . -name '*.go' -not -path './frontend/*')

tidy:
	cd $(PROJECT_ROOT) && $(GO) mod tidy

generate:
	cd $(PROJECT_ROOT) && $(WAILS) generate module

doctor:
	cd $(PROJECT_ROOT) && $(WAILS) doctor

frontend-install:
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-build:
	cd $(FRONTEND_DIR) && $(NPM) run build

frontend-dev:
	cd $(FRONTEND_DIR) && $(NPM) run dev

clean:
	rm -rf $(FRONTEND_DIR)/dist
