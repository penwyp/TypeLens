#!/usr/bin/env python3
import json
import os
import pty
import select
import signal
import subprocess
import threading
import time
from collections import deque
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


HOST = "127.0.0.1"
PORT = 17322
DEFAULT_TYPELESS_BIN = "/Applications/Typeless.app/Contents/MacOS/Typeless"
DEFAULT_LLDB_CMDS = "/Users/penwyp/Dat/TypeLens/scripts/typeless/typeless_lldb_common.txt"
READ_CHUNK = 65536
OUTPUT_LIMIT = 2000


class LLDBSession:
    def __init__(self):
        self.proc = None
        self.master_fd = None
        self.reader = None
        self.output = deque(maxlen=OUTPUT_LIMIT)
        self.lock = threading.Lock()
        self.seq = 0
        self.alive = False

    def _append_output(self, source, text):
        if not text:
            return
        with self.lock:
            self.seq += 1
            self.output.append({
                "seq": self.seq,
                "ts": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                "source": source,
                "text": text,
            })

    def _reader_loop(self):
        while self.alive and self.master_fd is not None:
            try:
                ready, _, _ = select.select([self.master_fd], [], [], 0.2)
                if not ready:
                    continue
                data = os.read(self.master_fd, READ_CHUNK)
                if not data:
                    break
                self._append_output("lldb", data.decode("utf-8", errors="replace"))
            except OSError as exc:
                self._append_output("bridge", f"[reader-error] {exc}\n")
                break
        self.alive = False

    def start(self, mode, target=None, cmd_file=DEFAULT_LLDB_CMDS):
        self.stop()
        master_fd, slave_fd = pty.openpty()
        args = ["lldb"]
        if cmd_file and os.path.exists(cmd_file):
            args.extend(["-s", cmd_file])
        if mode == "launch":
            if not target:
                target = DEFAULT_TYPELESS_BIN
            args.extend(["--", target])
        elif mode == "attach":
            if not target:
                raise ValueError("attach 模式需要 pid")
            args.extend(["-p", str(target)])
        else:
            raise ValueError(f"unknown mode: {mode}")

        env = os.environ.copy()
        env.setdefault("TERM", "xterm-256color")

        self.proc = subprocess.Popen(
            args,
            stdin=slave_fd,
            stdout=slave_fd,
            stderr=slave_fd,
            close_fds=True,
            env=env,
        )
        os.close(slave_fd)
        self.master_fd = master_fd
        self.alive = True
        self._append_output("bridge", f"[start] pid={self.proc.pid} mode={mode} target={target}\n")
        self.reader = threading.Thread(target=self._reader_loop, daemon=True)
        self.reader.start()
        time.sleep(0.4)
        return {
            "ok": True,
            "lldb_pid": self.proc.pid,
            "mode": mode,
            "target": target,
        }

    def stop(self):
        if self.proc is not None:
            try:
                if self.proc.poll() is None:
                    self.proc.terminate()
                    try:
                        self.proc.wait(timeout=2)
                    except subprocess.TimeoutExpired:
                        self.proc.kill()
            except ProcessLookupError:
                pass
        if self.master_fd is not None:
            try:
                os.close(self.master_fd)
            except OSError:
                pass
        self.proc = None
        self.master_fd = None
        self.alive = False

    def send_command(self, command, wait=0.8):
        if not self.proc or self.master_fd is None or self.proc.poll() is not None:
            raise RuntimeError("lldb 未运行")
        payload = command.rstrip("\n") + "\n"
        os.write(self.master_fd, payload.encode("utf-8"))
        self._append_output("client", payload)
        time.sleep(wait)
        return {"ok": True, "sent": command}

    def status(self):
        return {
            "running": bool(self.proc and self.proc.poll() is None),
            "lldb_pid": self.proc.pid if self.proc else None,
            "alive": self.alive,
            "buffered_events": len(self.output),
        }

    def read_output(self, after=None, limit=200):
        with self.lock:
            items = list(self.output)
        if after is not None:
            items = [item for item in items if item["seq"] > after]
        return items[-limit:]


SESSION = LLDBSession()


class Handler(BaseHTTPRequestHandler):
    server_version = "typeless-lldb-bridge/1.0"

    def _read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length > 0 else b"{}"
        return json.loads(raw.decode("utf-8"))

    def _write(self, code, payload):
        body = json.dumps(payload, ensure_ascii=False, indent=2).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.startswith("/health"):
            self._write(200, {"ok": True, "port": PORT, "status": SESSION.status()})
            return
        if self.path.startswith("/status"):
            self._write(200, SESSION.status())
            return
        if self.path.startswith("/output"):
            query = {}
            if "?" in self.path:
                for pair in self.path.split("?", 1)[1].split("&"):
                    if "=" in pair:
                        k, v = pair.split("=", 1)
                        query[k] = v
            after = int(query["after"]) if "after" in query and query["after"] else None
            limit = int(query.get("limit", "200"))
            self._write(200, {"items": SESSION.read_output(after=after, limit=limit)})
            return
        self._write(404, {"error": "not_found"})

    def do_POST(self):
        try:
            if self.path == "/launch":
                data = self._read_json()
                result = SESSION.start("launch", target=data.get("target"), cmd_file=data.get("cmd_file", DEFAULT_LLDB_CMDS))
                self._write(200, result)
                return
            if self.path == "/attach":
                data = self._read_json()
                result = SESSION.start("attach", target=data.get("pid"), cmd_file=data.get("cmd_file", DEFAULT_LLDB_CMDS))
                self._write(200, result)
                return
            if self.path == "/command":
                data = self._read_json()
                result = SESSION.send_command(data["command"], wait=float(data.get("wait", 0.8)))
                self._write(200, result)
                return
            if self.path == "/stop":
                SESSION.stop()
                self._write(200, {"ok": True})
                return
        except Exception as exc:
            self._write(500, {"ok": False, "error": str(exc)})
            return
        self._write(404, {"error": "not_found"})

    def log_message(self, format, *args):
        return


def main():
    server = ThreadingHTTPServer((HOST, PORT), Handler)

    def shutdown(*_args):
        SESSION.stop()
        server.shutdown()

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    print(f"[typeless-lldb-bridge] listening on http://{HOST}:{PORT}")
    print("[typeless-lldb-bridge] endpoints: GET /health /status /output, POST /launch /attach /command /stop")
    server.serve_forever()


if __name__ == "__main__":
    main()
