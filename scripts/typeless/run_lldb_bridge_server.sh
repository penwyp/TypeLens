#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_SCRIPT="$ROOT_DIR/scripts/typeless/lldb_bridge_server.py"

if [[ ! -f "$SERVER_SCRIPT" ]]; then
  echo "bridge server 不存在: $SERVER_SCRIPT" >&2
  exit 1
fi

echo "[typeless-lldb-bridge] starting"
echo "  script: $SERVER_SCRIPT"
echo "  port:   http://127.0.0.1:17322"
echo
echo "用法示例："
echo "  curl -X POST http://127.0.0.1:17322/launch -d '{\"target\":\"/Applications/Typeless.app/Contents/MacOS/Typeless\"}' -H 'content-type: application/json'"
echo "  curl -X POST http://127.0.0.1:17322/attach -d '{\"pid\":41795}' -H 'content-type: application/json'"
echo "  curl -X POST http://127.0.0.1:17322/command -d '{\"command\":\"continue\"}' -H 'content-type: application/json'"
echo "  curl http://127.0.0.1:17322/output?limit=200"

exec python3 "$SERVER_SCRIPT"
