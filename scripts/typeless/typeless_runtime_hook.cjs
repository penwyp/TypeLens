const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");
const process = require("node:process");
const { inspect } = require("node:util");

const PORT = Number(process.env.TYPELENS_TYPELESS_HOOK_PORT || 17321);
const MAX_EVENTS = 500;
const MAX_BODY_CHARS = 16 * 1024;
const START_TIME = new Date().toISOString();

const userDataDir = path.join(process.env.HOME || "", "Library", "Application Support", "Typeless");
const trackedFiles = new Set([
  path.join(userDataDir, "app-storage.json"),
  path.join(userDataDir, "user-data.json"),
  path.join(userDataDir, "app-settings.json"),
  path.join(userDataDir, "app-onboarding.json"),
]);

const state = {
  startedAt: START_TIME,
  pid: process.pid,
  port: PORT,
  events: [],
};

function pushEvent(kind, payload) {
  state.events.push({
    id: state.events.length + 1,
    ts: new Date().toISOString(),
    kind,
    payload,
  });
  if (state.events.length > MAX_EVENTS) {
    state.events.splice(0, state.events.length - MAX_EVENTS);
  }
}

function truncate(value, limit = MAX_BODY_CHARS) {
  if (typeof value !== "string") {
    return value;
  }
  if (value.length <= limit) {
    return value;
  }
  return `${value.slice(0, limit)}...<truncated ${value.length - limit} chars>`;
}

function normalizeHeaders(headers) {
  if (!headers) {
    return {};
  }
  if (typeof headers.entries === "function") {
    return Object.fromEntries(headers.entries());
  }
  if (Array.isArray(headers)) {
    return Object.fromEntries(headers);
  }
  return { ...headers };
}

async function readRequestBody(body) {
  if (body == null) {
    return null;
  }
  if (typeof body === "string") {
    return truncate(body);
  }
  if (body instanceof URLSearchParams) {
    return truncate(body.toString());
  }
  if (typeof FormData !== "undefined" && body instanceof FormData) {
    const fields = [];
    for (const [key, value] of body.entries()) {
      if (typeof value === "string") {
        fields.push({ key, type: "string", value: truncate(value, 1024) });
      } else {
        fields.push({
          key,
          type: value?.constructor?.name || "blob",
          name: value?.name || null,
          size: value?.size ?? null,
          mime: value?.type || null,
        });
      }
    }
    return { kind: "form-data", fields };
  }
  if (Buffer.isBuffer(body)) {
    return { kind: "buffer", size: body.length, preview: truncate(body.toString("utf8", 0, 1024)) };
  }
  if (body && typeof body === "object" && typeof body.arrayBuffer === "function") {
    try {
      const buf = Buffer.from(await body.arrayBuffer());
      return { kind: body?.constructor?.name || "binary", size: buf.length, preview: truncate(buf.toString("utf8", 0, 1024)) };
    } catch (error) {
      return { kind: body?.constructor?.name || "stream", error: String(error) };
    }
  }
  return inspect(body, { depth: 2, breakLength: 120 });
}

function maybeTrackFile(filePath) {
  const resolved = path.resolve(filePath);
  for (const tracked of trackedFiles) {
    if (resolved === tracked || resolved.startsWith(`${tracked}.`)) {
      return tracked;
    }
  }
  return null;
}

function patchFS() {
  const originalWriteFileSync = fs.writeFileSync;
  const originalRenameSync = fs.renameSync;
  const originalUnlinkSync = fs.unlinkSync;

  fs.writeFileSync = function patchedWriteFileSync(filePath, data, ...rest) {
    const tracked = maybeTrackFile(filePath);
    if (tracked) {
      pushEvent("fs.writeFileSync", {
        filePath: path.resolve(filePath),
        trackedFile: tracked,
        size: Buffer.isBuffer(data) ? data.length : Buffer.byteLength(String(data)),
      });
    }
    return originalWriteFileSync.call(this, filePath, data, ...rest);
  };

  fs.renameSync = function patchedRenameSync(oldPath, newPath, ...rest) {
    const tracked = maybeTrackFile(oldPath) || maybeTrackFile(newPath);
    if (tracked) {
      pushEvent("fs.renameSync", {
        oldPath: path.resolve(oldPath),
        newPath: path.resolve(newPath),
        trackedFile: tracked,
      });
    }
    return originalRenameSync.call(this, oldPath, newPath, ...rest);
  };

  fs.unlinkSync = function patchedUnlinkSync(filePath, ...rest) {
    const tracked = maybeTrackFile(filePath);
    if (tracked) {
      pushEvent("fs.unlinkSync", {
        filePath: path.resolve(filePath),
        trackedFile: tracked,
      });
    }
    return originalUnlinkSync.call(this, filePath, ...rest);
  };
}

function patchFetch() {
  if (typeof globalThis.fetch !== "function") {
    return;
  }
  const originalFetch = globalThis.fetch.bind(globalThis);
  globalThis.fetch = async function patchedFetch(input, init) {
    const url = typeof input === "string" ? input : input?.url;
    const method = init?.method || input?.method || "GET";
    const headers = normalizeHeaders(init?.headers || input?.headers);
    const requestBody = await readRequestBody(init?.body);

    pushEvent("fetch.request", {
      url,
      method,
      headers,
      body: requestBody,
    });

    const response = await originalFetch(input, init);
    const responseHeaders = normalizeHeaders(response.headers);
    let responseBody = null;
    try {
      responseBody = truncate(await response.clone().text());
    } catch (error) {
      responseBody = { error: String(error) };
    }

    pushEvent("fetch.response", {
      url,
      method,
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
      body: responseBody,
    });
    return response;
  };
}

function patchWebSocket() {
  if (typeof globalThis.WebSocket !== "function") {
    return;
  }
  const OriginalWebSocket = globalThis.WebSocket;
  globalThis.WebSocket = class HookedWebSocket extends OriginalWebSocket {
    constructor(url, protocols) {
      super(url, protocols);
      const socketUrl = typeof url === "string" ? url : String(url);
      pushEvent("ws.open", {
        url: socketUrl,
        protocols,
      });
      this.addEventListener("message", (event) => {
        pushEvent("ws.message", {
          url: socketUrl,
          data: truncate(typeof event.data === "string" ? event.data : inspect(event.data, { depth: 1 })),
        });
      });
      this.addEventListener("close", (event) => {
        pushEvent("ws.close", {
          url: socketUrl,
          code: event.code,
          reason: event.reason,
          wasClean: event.wasClean,
        });
      });
      this.addEventListener("error", () => {
        pushEvent("ws.error", { url: socketUrl });
      });
    }

    send(data) {
      pushEvent("ws.send", {
        url: this.url,
        data: truncate(typeof data === "string" ? data : inspect(data, { depth: 1 })),
      });
      return super.send(data);
    }
  };
}

function startServer() {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url || "/", `http://127.0.0.1:${PORT}`);
    res.setHeader("Content-Type", "application/json; charset=utf-8");

    if (req.method === "GET" && url.pathname === "/health") {
      res.end(JSON.stringify({ ok: true, startedAt: state.startedAt, pid: state.pid, port: state.port }));
      return;
    }

    if (req.method === "GET" && url.pathname === "/events") {
      const limit = Math.max(1, Math.min(Number(url.searchParams.get("limit") || 100), MAX_EVENTS));
      const kind = url.searchParams.get("kind");
      const events = kind ? state.events.filter((item) => item.kind === kind) : state.events;
      res.end(JSON.stringify({ total: events.length, items: events.slice(-limit) }));
      return;
    }

    if (req.method === "GET" && url.pathname === "/latest") {
      res.end(JSON.stringify(state.events[state.events.length - 1] || null));
      return;
    }

    if (req.method === "POST" && url.pathname === "/clear") {
      state.events.length = 0;
      res.end(JSON.stringify({ ok: true }));
      return;
    }

    res.statusCode = 404;
    res.end(JSON.stringify({ error: "not_found" }));
  });

  server.listen(PORT, "127.0.0.1", () => {
    pushEvent("hook.ready", {
      port: PORT,
      pid: process.pid,
      userDataDir,
    });
    process.stderr.write(`[typeless-hook] listening on http://127.0.0.1:${PORT}\n`);
  });
}

patchFS();
patchFetch();
patchWebSocket();
startServer();
