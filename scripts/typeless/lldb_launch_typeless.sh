#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TYPELESS_BIN="${TYPELESS_BIN:-/Applications/Typeless.app/Contents/MacOS/Typeless}"
LLDB_CMDS="$ROOT_DIR/scripts/typeless/typeless_lldb_common.txt"

if [[ ! -x "$TYPELESS_BIN" ]]; then
  echo "Typeless 可执行文件不存在或不可执行: $TYPELESS_BIN" >&2
  exit 1
fi

if [[ ! -f "$LLDB_CMDS" ]]; then
  echo "LLDB 命令文件不存在: $LLDB_CMDS" >&2
  exit 1
fi

cat <<EOF
[typeless-lldb] launch 模式
  bin:  $TYPELESS_BIN
  cmds: $LLDB_CMDS

会做的事:
  1. 启动原始 Typeless.app
  2. 预设 _getDeviceId / context-helper 相关断点
  3. 命中 _getDeviceId 时自动打印参数和回溯

进入 lldb 后常用:
  run
  continue
  bt
  btall
  iv
  image lookup -n _getDeviceId
EOF

exec lldb -s "$LLDB_CMDS" -- "$TYPELESS_BIN"
