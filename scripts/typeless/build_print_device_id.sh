#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$ROOT_DIR/scripts/typeless/print_device_id.c"
OUT="${TYPELESS_PRINT_DEVICE_ID_BIN:-/tmp/typeless-print-device-id}"

cc -O0 -g "$SRC" -o "$OUT" -ldl
echo "$OUT"
