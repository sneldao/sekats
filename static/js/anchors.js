import { verifyAnchor } from "./anchor-verifier.js";

const registry = document.getElementById("anchors-registry");
const rows = Array.from(registry.querySelectorAll(".anchor-row"));

async function verifyRow(row) {
  const button = row.querySelector(".verify-anchor");
  const status = row.querySelector('[role="status"]');
  const detail = row.querySelector(".verify-detail");
  if (button.disabled) return;
  button.disabled = true;
  status.className = "attest-badge verify-unavailable";
  status.textContent = "Checking…";
  const result = await verifyAnchor({
    draftID: row.dataset.draftId, docURL: row.dataset.docUrl,
    contract: row.dataset.contract, rpc: registry.dataset.rpc,
  });
  status.className = `attest-badge verify-${result.state}`;
  status.textContent = { match: "Match", mismatch: "Mismatch", "not-found": "Not found", unavailable: "Unavailable" }[result.state];
  detail.textContent = result.message + (result.docHash ? ` Local SHA-256: ${result.docHash}.` : "") +
    (result.onChain ? ` Getter: ${result.onChain}.` : "");
  button.disabled = false;
}

for (const row of rows) {
  row.querySelector(".verify-anchor").addEventListener("click", () => verifyRow(row));
  row.querySelector(".copy-call")?.addEventListener("click", async event => {
    const button = event.currentTarget;
    const command = row.querySelector(".cast-call");
    try {
      await navigator.clipboard.writeText(command.textContent);
      button.textContent = "Copied";
    } catch {
      const range = document.createRange();
      range.selectNodeContents(command);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      button.textContent = "Selected, copy manually";
    }
  });
}

async function verifyNext() {
  while (rows.length) await verifyRow(rows.shift());
}
await Promise.all(Array.from({ length: Math.min(4, rows.length) }, verifyNext));
