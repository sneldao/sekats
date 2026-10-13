// Minimal EIP-1193 backend: signs with the demo attester key via `cast`.
// Only for local demo-video recording — produces real on-chain signatures/txs.
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { readFileSync } from "node:fs";

const key = JSON.parse(readFileSync(process.env.HOME + "/.sekats/attester.json", "utf8"))[0];
const RPC = "https://rpc.monad.xyz";

const run = (args, input) => new Promise((res, rej) => {
  execFile("cast", args, { timeout: 30000 }, (e, out, err) => e ? rej(err + out + err) : res(out));
});

createServer(async (req, res) => {
  res.setHeader("access-control-allow-origin", "*");
  res.setHeader("access-control-allow-headers", "*");
  if (req.method === "OPTIONS") { res.end(); return; }
  let body = "";
  for await (const c of req) body += c;
  const { method, params = [] } = JSON.parse(body || "{}");
  res.setHeader("content-type", "application/json");
  try {
    if (method === "eth_requestAccounts" || method === "eth_accounts") {
      res.end(JSON.stringify({ result: [key.address] }));
    } else if (method === "eth_chainId") {
      res.end(JSON.stringify({ result: "0x8f" }));
    } else if (method === "wallet_switchEthereumChain" || method === "wallet_addEthereumChain") {
      res.end(JSON.stringify({ result: null }));
    } else if (method === "eth_signTypedData_v4") {
      const sig = (await run(["wallet", "sign", "--private-key", key.private_key, "--data", params[1]])).trim();
      res.end(JSON.stringify({ result: sig }));
    } else if (method === "eth_sendTransaction") {
      const tx = params[0];
      const out = await run(["send", "--private-key", key.private_key, "--rpc-url", RPC, "--json",
        tx.to, tx.data, ...(tx.value ? ["--value", tx.value] : [])]);
      const parsed = JSON.parse(out);
      res.end(JSON.stringify({ result: parsed.transactionHash }));
    } else {
      // proxy reads to public RPC
      const r = await fetch(RPC, { method: "POST", body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) });
      res.end(await r.text());
    }
  } catch (e) {
    res.statusCode = 500;
    res.end(JSON.stringify({ error: String(e).replace(key.private_key, "<redacted>") }));
  }
}).listen(9911, "127.0.0.1", () => console.log("signing proxy on :9911 as", key.address));
