import assert from "node:assert/strict";
import { createHash, webcrypto } from "node:crypto";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

const source = await readFile(new URL("./anchor-verifier.js", import.meta.url), "utf8");
const { verifyAnchor } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
const contract = `0x${"1".repeat(40)}`;
const options = { draftID: "draft-é", docURL: "https://console.example/doc", contract, rpc: "https://rpc.example" };
const doc = '{\n  "schema": "sekats-attestation/v1",\n  "draft_id": "draft-é"\n}';
const hash = value => `0x${createHash("sha256").update(value).digest("hex")}`;
Object.defineProperty(globalThis, "crypto", { value: webcrypto, configurable: true });

function fakeFetch(t, { bytes = doc, result = hash(bytes), error, docStatus = 200, rpcStatus = 200 } = {}) {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, init = {}) => {
    calls.push({ url, init });
    if (url === options.docURL) return new Response(bytes, { status: docStatus });
    assert.equal(url, options.rpc);
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: 1, result, error }), { status: rpcStatus });
  });
  return calls;
}

test("hashes exact bytes and UTF-8 draft ID, uses only the stable getter", async t => {
  const calls = fakeFetch(t);
  const result = await verifyAnchor({ ...options, docHash: `0x${"f".repeat(64)}`, draftKey: "server-key" });
  assert.equal(result.state, "match");
  assert.equal(result.docHash, hash(doc));
  assert.equal(result.draftKey, hash("sekats/draft/draft-é"));
  assert.equal(calls.length, 2);
  const request = JSON.parse(calls[1].init.body);
  assert.deepEqual(request, {
    jsonrpc: "2.0", id: 1, method: "eth_call",
    params: [{ to: contract, data: `0xdd649146${result.draftKey.slice(2)}` }, "latest"],
  });
  assert.equal(calls[1].init.credentials, "omit");
});

test("tampered bytes mismatch even if JSON is semantically unchanged", async t => {
  fakeFetch(t, { bytes: doc + "\n", result: hash(doc) });
  assert.equal((await verifyAnchor(options)).state, "mismatch");
});

test("document identity mismatch does not trust a matching digest", async t => {
  const calls = fakeFetch(t, { bytes: doc.replace("draft-é", "other-draft") });
  assert.equal((await verifyAnchor(options)).state, "mismatch");
  assert.equal(calls.length, 1);
});

for (const result of [`0x${"0".repeat(64)}`, "0x"]) {
  test(`missing anchor (${result.length} chars) is grey not-found`, async t => {
    fakeFetch(t, { result });
    assert.equal((await verifyAnchor(options)).state, "not-found");
  });
}

for (const [name, config] of [
  ["RPC error", { error: { code: -32000 } }],
  ["RPC HTTP error", { rpcStatus: 503 }],
  ["document missing", { docStatus: 404 }],
  ["malformed getter", { result: "0x1234" }],
  ["invalid document", { bytes: "not json" }],
]) {
  test(`${name} is unavailable, not a mismatch or fake zero`, async t => {
    fakeFetch(t, config);
    assert.equal((await verifyAnchor(options)).state, "unavailable");
  });
}

test("timeout aborts the document or RPC read", async t => {
  t.mock.method(globalThis, "fetch", (url, init) => new Promise((resolve, reject) => {
    init.signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
  }));
  const result = await verifyAnchor({ ...options, timeoutMs: 5 });
  assert.equal(result.state, "unavailable");
  assert.equal(result.message, "Verification timed out.");
});

test("missing contract does not contact the network", async t => {
  const calls = fakeFetch(t);
  assert.equal((await verifyAnchor({ ...options, contract: "" })).state, "unavailable");
  assert.equal(calls.length, 0);
});
