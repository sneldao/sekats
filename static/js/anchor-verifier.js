const getterSelector = "0xdd649146"; // Keccak-256("anchored(bytes32)")[:4]
const zeroHash = `0x${"0".repeat(64)}`;
const hex = bytes => `0x${Array.from(new Uint8Array(bytes), b => b.toString(16).padStart(2, "0")).join("")}`;

export async function verifyAnchor({ draftID, docURL, contract, rpc, timeoutMs = 8000 }) {
  if (!globalThis.crypto?.subtle) {
    return { state: "unavailable", message: "SHA-256 unavailable. Use HTTPS or localhost." };
  }
  if (!/^0x[0-9a-f]{40}$/i.test(contract) || !/^https?:\/\//i.test(rpc)) {
    return { state: "unavailable", message: "Contract or public RPC not configured." };
  }
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const docResponse = await fetch(docURL, { signal: controller.signal, cache: "no-store" });
    if (!docResponse.ok) throw new Error("Canonical document unavailable.");
    const bytes = await docResponse.arrayBuffer();
    const [docHash, draftKey] = await Promise.all([
      crypto.subtle.digest("SHA-256", bytes).then(hex),
      crypto.subtle.digest("SHA-256", new TextEncoder().encode(`sekats/draft/${draftID}`)).then(hex),
    ]);
    const doc = JSON.parse(new TextDecoder().decode(bytes));
    if (doc.draft_id !== draftID || !/^sekats-(attestation|approval)\/v\d+$/.test(doc.schema)) {
      return { state: "mismatch", message: "Document identity does not match this draft.", docHash, draftKey };
    }
    const response = await fetch(rpc, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "omit",
      signal: controller.signal,
      body: JSON.stringify({
        jsonrpc: "2.0", id: 1, method: "eth_call",
        params: [{ to: contract, data: getterSelector + draftKey.slice(2) }, "latest"],
      }),
    });
    if (!response.ok) throw new Error("RPC unavailable.");
    const result = await response.json();
    if (result.error || result.jsonrpc !== "2.0" || result.id !== 1) throw new Error("RPC response error.");
    const onChain = typeof result.result === "string" ? result.result.toLowerCase() : "";
    if (onChain === "0x" || onChain === zeroHash) {
      return { state: "not-found", message: "No anchor found at this contract and draft key.", docHash, draftKey, onChain };
    }
    if (!/^0x[0-9a-f]{64}$/.test(onChain)) throw new Error("Invalid contract getter response.");
    return {
      state: onChain === docHash ? "match" : "mismatch",
      message: onChain === docHash ? "Document matches Monad." : "Document does not match Monad.",
      docHash, draftKey, onChain,
    };
  } catch (error) {
    return {
      state: "unavailable",
      message: error.name === "AbortError" ? "Verification timed out." : "Verification unavailable: check the document, RPC and CORS access.",
    };
  } finally {
    clearTimeout(timer);
  }
}
