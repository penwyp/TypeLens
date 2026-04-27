#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LLDB_CMDS="$ROOT_DIR/scripts/typeless/typeless_lldb_common.txt"
PID="${1:-}"

if [[ ! -f "$LLDB_CMDS" ]]; then
  echo "LLDB 命令文件不存在: $LLDB_CMDS" >&2
  exit 1
fi

if [[ -z "$PID" ]]; then
  PID="$(pgrep -x Typeless | head -n 1 || true)"
fi

if [[ -z "$PID" ]]; then
  echo "没有找到运行中的 Typeless 进程。可手动传 PID：" >&2
  echo "  bash scripts/typeless/lldb_attach_typeless.sh <PID>" >&2
  exit 1
fi

cat <<EOF
[typeless-lldb] attach 模式
  pid:  $PID
  cmds: $LLDB_CMDS

会做的事:
  1. 附加到已运行的 Typeless 进程
  2. 预设 _getDeviceId / context-helper 相关断点
  3. 命中 _getDeviceId 时自动打印参数和回溯

进入 lldb 后常用:
  continue
  bt
  btall
  iv
  image lookup -n _getDeviceId
EOF

exec lldb -s "$LLDB_CMDS" -p "$PID"
