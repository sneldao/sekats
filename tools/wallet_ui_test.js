// Run through agent-browser eval in an isolated local fixture, never a wallet.
(async () => {
  const checks = [];
  const assert = (condition, name) => {
    if (!condition) throw new Error("FAIL: " + name);
    checks.push(name);
  };
  const waitFor = async condition => {
    for (let i = 0; i < 100; i++) {
      if (condition()) return;
      await new Promise(resolve => setTimeout(resolve, 10));
    }
    throw new Error("UI condition timed out");
  };
  const documentText = '{"schema":"sekats-approval/v2","entry":{"rule":"fixture only"}}';
  const hashBytes = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(documentText));
  const hash = "0x" + Array.from(new Uint8Array(hashBytes), x => x.toString(16).padStart(2, "0")).join("");
  let mode = "success", added = false, walletCalls = [], submissions = [];
  const data = {
    document: documentText, doc_sha256: hash,
    typed_data: {
      types: {
        EIP712Domain: [{ name: "name", type: "string" }, { name: "version", type: "string" },
          { name: "chainId", type: "uint256" }, { name: "verifyingContract", type: "address" }],
        Approval: [{ name: "draftKey", type: "bytes32" }, { name: "docHash", type: "bytes32" }]
      },
      primaryType: "Approval",
      domain: { name: "SekatsAttestations", version: "2", chainId: "143", verifyingContract: "0x1111111111111111111111111111111111111111" },
      message: { draftKey: "0x" + "11".repeat(32), docHash: hash }
    },
    chain: {
      chainId: "0x8f", chainName: "Monad fixture",
      nativeCurrency: { name: "Monad", symbol: "MON", decimals: 18 },
      rpcUrls: ["https://example.invalid"], blockExplorerUrls: ["https://example.invalid"]
    }
  };
  window.fetch = async (url, options = {}) => {
    if (url.endsWith("/typed-data")) {
      const response = structuredClone(data);
      if (mode === "tampered") response.document += " ";
      return new Response(JSON.stringify(response));
    }
    submissions.push({ url, body: JSON.parse(options.body) });
    if (mode === "relay-failed") {
      return new Response(JSON.stringify({ error: "fixture relay failed", tx_url: "https://example.invalid/tx/fixture" }), { status: 502 });
    }
    return new Response(JSON.stringify({
      status: "anchored", attester: "fixture-approver",
      tx_url: "https://example.invalid/tx/fixture", policy_sha256: hash
    }));
  };
  const wallet = {
    request: async ({ method, params }) => {
      walletCalls.push({ method, params });
      if (method === "eth_requestAccounts") return ["0x2222222222222222222222222222222222222222"];
      if (method === "wallet_switchEthereumChain") {
        if (!added) throw Object.assign(new Error("unknown chain"), { code: 4902 });
        return null;
      }
      if (method === "wallet_addEthereumChain") { added = true; return null; }
      if (method === "eth_chainId") return mode === "wrong-chain" ? "0x1" : "0x8f";
      if (method === "eth_signTypedData_v4") {
        if (mode === "rejected") throw Object.assign(new Error("fixture rejection"), { code: 4001 });
        assert(JSON.parse(params[1]).message.docHash === hash, "signs server's verified document hash");
        return "public-signature-fixture-not-a-key";
      }
      throw new Error("unexpected wallet method");
    }
  };
  const button = document.querySelector("[data-wallet-approve]");
  const status = document.querySelector("[data-wallet-status]");
  delete window.ethereum;
  button.click();
  await waitFor(() => status.textContent.includes("Wallet missing"));
  assert(submissions.length === 0, "wallet-missing state makes no submission");
  window.ethereum = wallet;
  mode = "rejected";
  button.click();
  await waitFor(() => document.querySelector("dialog[open]"));
  let dialog = document.querySelector("dialog");
  assert(dialog.querySelector(".wallet-document").textContent === documentText, "shows exact canonical bytes before signing");
  assert(dialog.querySelector("[data-sign]").disabled && walletCalls.length === 0, "requires explicit document review before wallet requests");
  dialog.querySelector("input").checked = true;
  dialog.querySelector("input").dispatchEvent(new Event("change"));
  dialog.querySelector("[data-sign]").click();
  await waitFor(() => dialog.querySelector("[data-sign-status]").textContent.includes("rejected"));
  assert(submissions.length === 0, "rejected signature never posts");
  assert(walletCalls.some(call => call.method === "wallet_addEthereumChain"), "adds missing Monad chain");
  mode = "success";
  dialog.querySelector("[data-sign]").click();
  await waitFor(() => status.textContent.includes("Approval anchored"));
  await waitFor(() => !document.querySelector("dialog"));
  assert(submissions.length === 1 && Object.keys(submissions[0].body).join(",") === "signature", "posts only signature, never a client hash");
  assert(status.querySelector("a").href === "https://example.invalid/tx/fixture", "shows approval transaction link");
  assert(button.disabled && !document.querySelector("dialog"), "successful approval closes dialog and disables repeat button");
  for (const scenario of ["tampered", "wrong-chain", "relay-failed"]) {
    mode = scenario;
    button.disabled = false;
    status.textContent = "";
    const previousSubmissions = submissions.length;
    button.click();
    if (scenario === "tampered") {
      await waitFor(() => status.textContent.includes("does not match"));
      assert(!document.querySelector("dialog") && submissions.length === previousSubmissions, "rejects mismatched canonical document before signing");
      continue;
    }
    await waitFor(() => document.querySelector("dialog[open]"));
    dialog = document.querySelector("dialog");
    const consent = dialog.querySelector("input");
    consent.checked = true; consent.dispatchEvent(new Event("change"));
    dialog.querySelector("[data-sign]").click();
    const expected = scenario === "wrong-chain" ? "wrong chain" : "fixture relay failed";
    await waitFor(() => status.textContent.includes(expected));
    if (scenario === "wrong-chain") {
      assert(submissions.length === previousSubmissions, "wrong-chain state does not submit");
    }
    assert(!dialog.querySelector("[data-cancel]").disabled, "failed flow allows safe retry or cancel");
    dialog.close();
    await waitFor(() => !document.querySelector("dialog"));
  }
  mode = "success";
  const applied = document.querySelector("[data-policy-applied]");
  applied.click();
  await waitFor(() => status.textContent.includes("Policy anchored"));
  assert(Object.keys(submissions.at(-1).body).length === 0, "policy reporting never sends a client policy hash or path");
  return { status: "passed", checks };
})();
