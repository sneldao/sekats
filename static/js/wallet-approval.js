/* Vanilla EIP-1193/EIP-712 wallet flow. No dependencies or build step. */
(() => {
  "use strict";
  async function requestJSON(url, options) {
    const response = await fetch(url, { credentials: "same-origin", ...options });
    const text = await response.text();
    let body;
    try { body = JSON.parse(text); } catch (_) { body = null; }
    if (!response.ok) {
      const error = new Error((body && body.error) || text || "Request failed.");
      error.result = body;
      throw error;
    }
    return body;
  }
  function post(url, value) {
    return requestJSON(url, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value)
    });
  }
  function statusNode(button) {
    return button.closest(".draft-card").querySelector("[data-wallet-status]");
  }
  function showResult(node, text, result) {
    node.replaceChildren(document.createTextNode(text));
    // Never insert server strings as HTML, including explorer URLs.
    if (result && result.tx_url && /^https?:\/\//.test(result.tx_url)) {
      const link = document.createElement("a");
      link.href = result.tx_url; link.target = "_blank"; link.rel = "noopener";
      link.textContent = "View transaction";
      node.append(" ", link);
    } else if (result && result.tx_hash) {
      node.append(" Transaction: ", result.tx_hash);
    }
  }
  function explain(error) {
    if (error.code === 4001 || (error.error && error.error.code === 4001)) {
      return "Wallet request rejected. Nothing was submitted by this browser.";
    }
    return error.message || "Wallet approval failed.";
  }
  async function switchChain(wallet, chain) {
    try {
      await wallet.request({ method: "wallet_switchEthereumChain", params: [{ chainId: chain.chainId }] });
    } catch (error) {
      if (error.code !== 4902 && !(error.error && error.error.code === 4902)) throw error;
      await wallet.request({ method: "wallet_addEthereumChain", params: [chain] });
      await wallet.request({ method: "wallet_switchEthereumChain", params: [{ chainId: chain.chainId }] });
    }
    const actual = await wallet.request({ method: "eth_chainId" });
    if (BigInt(actual) !== BigInt(chain.chainId)) throw new Error("Wallet is on the wrong chain.");
  }
  async function approve(button) {
    const status = statusNode(button);
    const wallet = window.ethereum;
    if (!wallet) {
      showResult(status, "Wallet missing. Install an Ethereum-compatible wallet such as MetaMask, then reload.");
      return;
    }
    button.disabled = true;
    let dialog;
    try {
      const base = "/promotions/" + encodeURIComponent(button.dataset.walletApprove);
      const data = await requestJSON(base + "/typed-data");
      const bytes = new TextEncoder().encode(data.document);
      const digest = await crypto.subtle.digest("SHA-256", bytes);
      const hash = "0x" + Array.from(new Uint8Array(digest), x => x.toString(16).padStart(2, "0")).join("");
      if (hash.toLowerCase() !== data.typed_data.message.docHash.toLowerCase() ||
          hash.toLowerCase() !== data.doc_sha256.toLowerCase()) {
        throw new Error("Canonical document does not match the signing hash.");
      }
      if (BigInt(data.chain.chainId) !== BigInt(data.typed_data.domain.chainId)) {
        throw new Error("Wallet chain does not match the signing domain.");
      }
      dialog = document.createElement("dialog");
      dialog.className = "wallet-dialog";
      dialog.innerHTML = '<h2>Review the exact approval document</h2>' +
        '<p>Your wallet displays hashes, not this policy. Read this document before signing.</p>' +
        '<p class="wallet-domain"></p><pre class="wallet-document"></pre>' +
        '<label class="wallet-consent"><input type="checkbox"> I reviewed this exact document and approve its policy edit.</label>' +
        '<div class="wallet-actions"><button type="button" class="btn btn-accept" data-sign disabled>Sign approval</button>' +
        '<button type="button" class="btn" data-cancel>Cancel</button></div>' +
        '<p data-sign-status role="status" aria-live="polite"></p>';
      dialog.querySelector(".wallet-document").textContent = data.document;
      dialog.querySelector(".wallet-domain").textContent =
        "Chain " + data.typed_data.domain.chainId + " · Contract " +
        data.typed_data.domain.verifyingContract + " · Doc SHA-256 " + hash;
      const consent = dialog.querySelector("input");
      const sign = dialog.querySelector("[data-sign]");
      const cancel = dialog.querySelector("[data-cancel]");
      let anchored = false;
      consent.addEventListener("change", () => { sign.disabled = !consent.checked; });
      cancel.addEventListener("click", () => dialog.close());
      dialog.addEventListener("close", () => { dialog.remove(); button.disabled = anchored; });
      sign.addEventListener("click", async () => {
        sign.disabled = true; consent.disabled = true; cancel.disabled = true;
        const progress = dialog.querySelector("[data-sign-status]");
        // Prevent dismissing while the wallet/relay outcome is uncertain.
        const preventCancel = event => event.preventDefault();
        dialog.addEventListener("cancel", preventCancel);
        try {
          progress.textContent = "Connect your wallet and confirm the chain.";
          const accounts = await wallet.request({ method: "eth_requestAccounts" });
          if (!accounts.length) throw new Error("No wallet account selected.");
          await switchChain(wallet, data.chain);
          progress.textContent = "Sign the approval. Signing itself does not spend funds.";
          const signature = await wallet.request({
            method: "eth_signTypedData_v4",
            params: [accounts[0], JSON.stringify(data.typed_data)]
          });
          progress.textContent = "Signature received. Waiting for the service relay receipt.";
          const result = await post(base + "/signature", { signature });
          showResult(status, "Approval anchored by " + result.attester + ". Refresh to download the patch.", result);
          anchored = true;
          dialog.close();
        } catch (error) {
          progress.textContent = explain(error);
          showResult(status, explain(error), error.result);
          sign.disabled = false; consent.disabled = false; cancel.disabled = false;
        } finally {
          dialog.removeEventListener("cancel", preventCancel);
        }
      });
      document.body.append(dialog);
      dialog.showModal();
    } catch (error) {
      if (dialog) dialog.remove();
      button.disabled = false;
      showResult(status, explain(error), error.result);
    }
  }
  document.addEventListener("click", async event => {
    const approveButton = event.target.closest("[data-wallet-approve]");
    if (approveButton) { await approve(approveButton); return; }
    const applied = event.target.closest("[data-policy-applied]");
    if (!applied) return;
    const status = statusNode(applied);
    applied.disabled = true;
    try {
      showResult(status, "Hashing the resulting policy file and waiting for the lineage receipt.");
      const result = await post("/promotions/" + encodeURIComponent(applied.dataset.policyApplied) + "/applied", {});
      showResult(status, "Policy anchored: sha256:" + result.policy_sha256 + ". Refresh to see history.", result);
    } catch (error) {
      showResult(status, explain(error), error.result);
    } finally {
      applied.disabled = false;
    }
  });
})();
