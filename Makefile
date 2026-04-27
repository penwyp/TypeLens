SHELL := /bin/zsh

PROJECT_ROOT := $(abspath .)
FRONTEND_DIR := $(PROJECT_ROOT)/frontend
BUILD_DIR := $(PROJECT_ROOT)/build/bin
APP_NAME := TypeLens
CLI_NAME := typelens
APP_BUNDLE := $(BUILD_DIR)/$(APP_NAME).app
CLI_BINARY := $(BUILD_DIR)/$(CLI_NAME)
GO ?= /Users/penwyp/.local/share/mise/installs/go/$(shell ls /Users/penwyp/.local/share/mise/installs/go | sort -V | tail -n 1)/bin/go
WAILS ?= /Users/penwyp/.local/bin/wails
NPM ?= npm
INSTALL_APP_DIR ?= $(shell if [ -w /Applications ]; then echo /Applications; else echo $$HOME/Applications; fi)
INSTALL_BIN_DIR ?= $(HOME)/.local/bin
INSTALL_APP_PATH := $(INSTALL_APP_DIR)/$(APP_NAME).app
INSTALL_CLI_PATH := $(INSTALL_BIN_DIR)/$(CLI_NAME)

CLI_ARGS ?=
RUN_ARGS ?=

.PHONY: help dev build package cli-build cli desktop test fmt tidy generate doctor frontend-install frontend-build frontend-dev install install-app install-cli upgrade uninstall clean

help:
	@echo "TypeLens Make Targets"
	@echo ""
	@echo "  make dev                Run Wails desktop in development mode"
	@echo "  make build              Build all Go packages"
	@echo "  make package            Build desktop application via Wails"
	@echo "  make cli-build          Build CLI binary into build/bin/typelens"
	@echo "  make desktop            Run desktop entry with 'go run .'"
	@echo "  make cli CLI_ARGS='...' Run CLI entry, e.g. make cli CLI_ARGS='dict list'"
	@echo "  make install            Install desktop app and CLI"
	@echo "  make upgrade            Rebuild and overwrite installed app and CLI"
	@echo "  make uninstall          Remove installed app and CLI"
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
	cd $(PROJECT_ROOT) && $(WAILS) build -clean

cli-build:
	mkdir -p $(BUILD_DIR)
	cd $(PROJECT_ROOT) && $(GO) build -o $(CLI_BINARY) ./cmd/typelens

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

install: install-app install-cli
	@echo "Installed desktop app to $(INSTALL_APP_PATH)"
	@echo "Installed CLI to $(INSTALL_CLI_PATH)"

install-app: package
	mkdir -p "$(INSTALL_APP_DIR)"
	rm -rf "$(INSTALL_APP_PATH)"
	cp -R "$(APP_BUNDLE)" "$(INSTALL_APP_PATH)"

install-cli: cli-build
	mkdir -p "$(INSTALL_BIN_DIR)"
	install -m 0755 "$(CLI_BINARY)" "$(INSTALL_CLI_PATH)"

upgrade: install

uninstall:
	rm -rf "$(INSTALL_APP_PATH)"
	rm -f "$(INSTALL_CLI_PATH)"
	@echo "Removed $(INSTALL_APP_PATH)"
	@echo "Removed $(INSTALL_CLI_PATH)"

clean:
	rm -rf $(FRONTEND_DIR)/dist
