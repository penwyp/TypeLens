const fs = require("fs");
const crypto = require("crypto");
const path = require("path");

const DEFAULT_API_HOST = "https://api.typeless.com";
const APP_NAME = "Typeless";
const APP_VERSION = "mac_1.2.1";
const SECURITY_SECRET = "5f69d2e7b648a41e027807ad5dd1d679f5df194ea43c2d47aea317b9";
const AUTHORIZATION_KEY = "a8ceffb90069eac13d3ecb057da340054e5936bae788cd56bd1a4e72";
const HUB_FILE_URL = "file:///Applications/Typeless.app/Contents/Resources/app.asar/dist/renderer/hub.html";

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

function defaultUserDataPath() {
  return path.join(process.env.HOME, "Library", "Application Support", "Typeless", "user-data.json");
}

function decryptUserData(userDataPath) {
  const data = fs.readFileSync(userDataPath || defaultUserDataPath());
  const platformArch = `${process.platform}-${process.arch}`;
  const digest = crypto.createHash("sha256").update(platformArch).digest("hex");
  const key = crypto.pbkdf2Sync(`${digest}${APP_NAME}`, "typeless-user-service", 10000, 32, "sha256");
  const iv = data.subarray(0, 16);
  const decipherKey = crypto.pbkdf2Sync(key, iv.toString(), 10000, 32, "sha512");
  const decipher = crypto.createDecipheriv("aes-256-cbc", decipherKey, iv);
  const plain = Buffer.concat([decipher.update(data.subarray(17)), decipher.final()]);
  const store = JSON.parse(plain.toString("utf8"));
  return JSON.parse(store.userData);
}

function evpBytesToKey(password, salt, keyLen, ivLen) {
  let out = Buffer.alloc(0);
  let prev = Buffer.alloc(0);
  while (out.length < keyLen + ivLen) {
    const md5 = crypto.createHash("md5");
    md5.update(prev);
    md5.update(Buffer.isBuffer(password) ? password : Buffer.from(password, "utf8"));
    md5.update(salt);
    prev = md5.digest();
    out = Buffer.concat([out, prev]);
  }
  return {
    key: out.subarray(0, keyLen),
    iv: out.subarray(keyLen, keyLen + ivLen),
  };
}

function encryptOpenSSL(plaintext, passphrase) {
  const salt = crypto.randomBytes(8);
  const { key, iv } = evpBytesToKey(passphrase, salt, 32, 16);
  const cipher = crypto.createCipheriv("aes-256-cbc", key, iv);
  const encrypted = Buffer.concat([cipher.update(Buffer.from(plaintext, "utf8")), cipher.final()]);
  return Buffer.concat([Buffer.from("Salted__"), salt, encrypted]).toString("base64");
}

function randomDigits(length) {
  const min = 10 ** (length - 1);
  const max = 10 ** length;
  return String(Math.floor(min + Math.random() * (max - min)));
}

function buildHeaders(user, pathname) {
  const timestamp = Date.now();
  const signStr = `${timestamp}:${APP_VERSION}:${pathname}:${user.user_id}`;
  const sha1SecretKey = `${timestamp}:${SECURITY_SECRET}`;
  const sha1Hash = crypto.createHmac("sha1", sha1SecretKey).update(signStr).digest("hex");
  const p = crypto.createHash("sm3").update(`${timestamp}:${sha1Hash}:${SECURITY_SECRET}`).digest("hex");
  const xAuthorization = encryptOpenSSL(
    JSON.stringify({
      "X-Env": "prod",
      "X-Client-Domain": HUB_FILE_URL,
      "X-Client-Path": HUB_FILE_URL,
      "X-Random": randomDigits(6),
      t: timestamp,
      p,
      d: "UNKNOWN",
      "3c86e26ccbb7274f752e7d868a1541ebfb7f37e7": { a: "" },
    }),
    AUTHORIZATION_KEY,
  );
  return {
    Authorization: `Bearer ${user.refresh_token}`,
    Accept: "application/json",
    "Content-Type": "application/json",
    "X-Browser-Name": "UNKNOWN",
    "X-Browser-Version": "UNKNOWN",
    "X-Browser-Major": "UNKNOWN",
    "X-App-Version": APP_VERSION,
    "X-Authorization": xAuthorization,
  };
}

async function requestAPI(request) {
  const user = decryptUserData(request.userDataPath);
  const apiHost = (request.apiHost || DEFAULT_API_HOST).replace(/\/+$/, "");
  let pathname = "";
  let method = "GET";
  let body;
  if (request.action === "list") {
    pathname = "/user/dictionary/list";
  } else if (request.action === "add") {
    pathname = "/user/dictionary/add";
    method = "POST";
    body = JSON.stringify({ term: request.term });
  } else if (request.action === "delete") {
    pathname = "/user/dictionary/delete";
    method = "POST";
    body = JSON.stringify({ user_dictionary_id: request.id });
  } else {
    throw new Error(`unknown action: ${request.action}`);
  }

  const url = new URL(apiHost + pathname);
  if (request.action === "list") {
    url.searchParams.set("offset", String(request.offset || 0));
    url.searchParams.set("size", String(request.size || 150));
  }

  const response = await fetch(url, {
    method,
    headers: buildHeaders(user, pathname),
    body,
    signal: AbortSignal.timeout(Math.max(1000, request.timeoutMs || 15000)),
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${text}`);
  }
  return JSON.parse(text);
}

async function main() {
  const raw = await readStdin();
  const request = JSON.parse(raw);
  const response = await requestAPI(request);
  process.stdout.write(JSON.stringify(response));
}

main().catch((error) => {
  process.stderr.write(error.stack || String(error));
  process.exit(1);
});
