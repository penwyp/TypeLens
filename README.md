# TypeLens

English documentation. For Chinese documentation, see [README.zh-CN.md](./README.zh-CN.md).

TypeLens is a macOS desktop and CLI companion for Typeless. It focuses on two workflows:

- managing the remote Typeless dictionary with a faster local UX
- searching Typeless transcript history and copying useful text quickly

The desktop app is built with Wails + React + TypeScript. The CLI is built with Cobra.

## What It Does

### Dictionary management

- List all remote Typeless dictionary entries
- Add and delete dictionary entries
- Import dictionary entries from a plain text file
  - expected format: one term per line
- Reset the dictionary against built-in defaults or a custom file
- Export the current dictionary to a `.txt` file
- Show pending auto-import terms alongside synced remote terms

### Auto import

- Scan Codex, Claude, and additional user-provided directories for candidate terms
- Parse `history.jsonl` and related `*.jsonl` files
- Preview candidate terms before importing
- Select/deselect terms before confirmation
- Persist accepted terms locally, then sync them to the remote Typeless dictionary in the background
- Stream real-time scan and sync logs in the desktop UI
  - estimated files to scan
  - scanned files progress
  - total extracted text/messages
  - raw candidate count
  - final candidate count

### History browsing

- Query recent Typeless transcript history
- Filter by keyword or regex
- Choose context mode:
  - `all`
  - `frontmost`
  - `latest`
- Sort by newest-first or oldest-first
- Copy a transcript entry with one click

### Local-first desktop UX

- Show cached dictionary and history immediately on startup
- Refresh in the background without interrupting the page
- Persist cache on disk instead of browser storage

## Desktop Features

- Sidebar with `Dictionary` and `History`
- Import dialog with tabs:
  - `Import File`
  - `Auto Import`
- Context menu refresh on dictionary items
- Keyboard shortcuts:
  - `Cmd/Ctrl + F`: jump to history search
  - `Cmd + W`: hide the desktop window

## CLI Features

Available commands:

- `typelens dict list`
- `typelens dict add <term>`
- `typelens dict import <file> [--dry-run] [--concurrency N]`
- `typelens dict delete --id <id>`
- `typelens dict clear --yes [--concurrency N]`
- `typelens dict reset --yes [--file <path>] [--concurrency N]`
- `typelens history [--limit N] [--keyword text] [--regex expr] [--context frontmost|latest|all] [--no-copy] [--full]`
- `typelens auto-import`

## Runtime Requirements

TypeLens currently assumes a macOS Typeless installation and an authenticated Typeless account.

It reads Typeless local data from:

- `~/Library/Application Support/Typeless/user-data.json`
- `~/Library/Application Support/Typeless/typeless.db`

It also uses these local files:

- cache: `~/.typelens/cache.json`
- pending auto-import state: `~/Library/Application Support/TypeLens/auto-import-pending.json`
- default export directory: `~/Downloads`

## Project Structure

- `app.go`: Wails desktop bindings
- `internal/service/`: application services, cache store, auto-import orchestration
- `internal/cli/`: CLI entrypoints and commands
- `pkg/typeless/`: Typeless API, history, import/export, auth, auto-import primitives
- `frontend/src/`: React desktop UI

## Development

### Prerequisites

- Go `1.26+`
- Node.js and npm
- Wails CLI
- macOS with Typeless installed if you want to use real local Typeless data

### Run desktop development mode

```bash
wails dev
```

### Run frontend-only build

```bash
cd frontend
npm run build
```

### Run tests

```bash
go test ./...
```

### Build the desktop app

```bash
wails build
```

## Install And Update

TypeLens now supports a local install flow through `make`.

### Install

```bash
make install
```

This does two things:

- builds the desktop app and installs `TypeLens.app`
- builds the CLI and installs `typelens`

Default install locations on macOS:

- desktop app:
  - `/Applications/TypeLens.app` when `/Applications` is writable
  - otherwise `~/Applications/TypeLens.app`
- CLI:
  - `~/.local/bin/typelens`

### Update

```bash
make upgrade
```

`make upgrade` rebuilds the app and CLI, then overwrites the installed copies in place.

### Uninstall

```bash
make uninstall
```

This removes:

- the installed `TypeLens.app`
- the installed `typelens` CLI binary

### Custom install paths

You can override install paths when needed:

```bash
make install INSTALL_APP_DIR=~/Applications INSTALL_BIN_DIR=/usr/local/bin
```

## Notes

- Dictionary import files are plain text files with one term per line.
- Auto-import source labels are a UI hint only. Import behavior is text-driven; the source label is not required for dictionary semantics.
- Background sync may temporarily keep pending words visible until remote sync completes.

For Chinese documentation, see [README.zh-CN.md](./README.zh-CN.md).
