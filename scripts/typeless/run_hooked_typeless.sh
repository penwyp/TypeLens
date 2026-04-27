#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HOOK_SCRIPT="$ROOT_DIR/scripts/typeless/typeless_runtime_hook.cjs"
TYPELESS_BIN="${TYPELESS_BIN:-/Applications/Typeless.app/Contents/MacOS/Typeless}"
TYPELESS_APP="${TYPELESS_APP:-/Applications/Typeless.app}"
HOOK_PORT="${TYPELENS_TYPELESS_HOOK_PORT:-17321}"
PATCH_APP_DIR="${TYPELENS_TYPELESS_PATCH_APP_DIR:-/tmp/Typeless-hooked.app}"
PATCH_WORK_DIR="${TYPELENS_TYPELESS_PATCH_WORK_DIR:-/tmp/typeless-hooked-work}"
PATCH_MAIN_REL="dist/main/index.js"
PATCH_IMPORT="import \"${HOOK_SCRIPT}\";"

if [[ ! -f "$HOOK_SCRIPT" ]]; then
  echo "hook script 不存在: $HOOK_SCRIPT" >&2
  exit 1
fi

if [[ ! -d "$TYPELESS_APP" ]]; then
  echo "Typeless app 不存在: $TYPELESS_APP" >&2
  exit 1
fi

export TYPELENS_TYPELESS_HOOK_PORT="$HOOK_PORT"

copy_app_if_needed() {
  local src_mtime dst_mtime
  src_mtime="$(stat -f '%m' "$TYPELESS_APP")"
  dst_mtime=""
  if [[ -d "$PATCH_APP_DIR" ]]; then
    dst_mtime="$(stat -f '%m' "$PATCH_APP_DIR" 2>/dev/null || true)"
  fi

  if [[ ! -d "$PATCH_APP_DIR" || "$src_mtime" != "$dst_mtime" ]]; then
    rm -rf "$PATCH_APP_DIR"
    mkdir -p "$(dirname "$PATCH_APP_DIR")"
    cp -R "$TYPELESS_APP" "$PATCH_APP_DIR"
  fi
}

patch_asar() {
  local app_asar patched_asar extracted_main
  app_asar="$PATCH_APP_DIR/Contents/Resources/app.asar"
  patched_asar="$PATCH_WORK_DIR/app.asar"
  extracted_main="$PATCH_WORK_DIR/extracted/$PATCH_MAIN_REL"

  rm -rf "$PATCH_WORK_DIR"
  mkdir -p "$PATCH_WORK_DIR"
  npx -y asar extract "$app_asar" "$PATCH_WORK_DIR/extracted"

  if [[ ! -f "$extracted_main" ]]; then
    echo "解包后找不到主进程入口: $extracted_main" >&2
    exit 1
  fi

  if ! grep -Fq "$PATCH_IMPORT" "$extracted_main"; then
    python3 - <<'PY' "$extracted_main" "$PATCH_IMPORT"
from pathlib import Path
import sys
target = Path(sys.argv[1])
inject = sys.argv[2]
original = target.read_text()
target.write_text(f"{inject}\n{original}")
PY
  fi

  npx -y asar pack "$PATCH_WORK_DIR/extracted" "$patched_asar"
  cp "$patched_asar" "$app_asar"
}

copy_app_if_needed
patch_asar

PATCHED_BIN="$PATCH_APP_DIR/Contents/MacOS/Typeless"

if [[ ! -x "$PATCHED_BIN" ]]; then
  echo "patched Typeless 可执行文件不存在或不可执行: $PATCHED_BIN" >&2
  exit 1
fi

cat <<EOF
[typeless-hook] 启动 Typeless 副本并注入运行时 Hook
  src:  $TYPELESS_APP
  app:  $PATCH_APP_DIR
  bin:  $PATCHED_BIN
  hook: $HOOK_SCRIPT
  port: http://127.0.0.1:$HOOK_PORT

可用接口:
  GET  /health
  GET  /latest
  GET  /events?limit=50
  GET  /events?kind=fetch.request
  GET  /events?kind=fetch.response
  GET  /events?kind=ws.open
  GET  /events?kind=ws.send
  GET  /events?kind=ws.message
  GET  /events?kind=fs.renameSync
  POST /clear
EOF

exec "$PATCHED_BIN" "$@"
