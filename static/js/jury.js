// Permissionless juror verdicts: the visitor's own wallet sends
// attest(bytes32,uint8) to SekatsJury — no relay, no server key. Tallies,
// the jury-box counter, and "your verdict" state are all read back through
// the public RPC, never trusted from the server.
const list = document.getElementById("review-list");
const config = {
  contract: list.dataset.juryContract,
  rpc: list.dataset.juryRpc,
  chainId: Number(list.dataset.juryChain),
  explorer: list.dataset.juryExplorer,
};
const ATTEST_SELECTOR = "0xd1143450"; // attest(bytes32,uint8)
const TALLY_SELECTOR = "0xf6e22eb5"; // tally(bytes32)
const VERDICT_OF_SELECTOR = "0x73feb97c"; // verdictOf(bytes32,address)
const pad32 = hex => hex.replace(/^0x/, "").padStart(64, "0");
const shortAddr = a => a.slice(0, 6) + "…" + a.slice(-4);

const CHAIN_PARAMS = id => ({
  chainId: "0x" + id.toString(16),
  chainName: "Monad",
  nativeCurrency: { name: "MON", symbol: "MON", decimals: 18 },
  rpcUrls: [config.rpc],
  blockExplorerUrls: [config.explorer],
});

async function rpc(method, params) {
  const response = await fetch(config.rpc, {
    method: "POST", headers: { "Content-Type": "application/json" }, credentials: "omit",
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
  });
  if (!response.ok) throw new Error("RPC unavailable");
  const result = await response.json();
  if (result.error) throw new Error("RPC response error");
  return result.result;
}

async function readTally(row) {
  const raw = await rpc("eth_call", [{ to: config.contract, data: TALLY_SELECTOR + pad32(row.dataset.juryKey) }, "latest"]);
  if (!raw || raw === "0x") return [0n, 0n];
  return [BigInt("0x" + raw.slice(2, 66)), BigInt("0x" + raw.slice(66, 130))];
}

async function refreshTally(row) {
  const el = row.querySelector("[data-jury-tally]");
  try {
    const [agent, shadow] = await readTally(row);
    el.textContent = `agent ${agent} · shadow ${shadow}`;
    el.dataset.agent = agent; el.dataset.shadow = shadow;
  } catch {
    el.textContent = "tally unavailable";
  }
}

// The jury box sums the tallies this page already reads — the queue is the
// jury's universe. (eth_getLogs is range-limited on the public RPC.)
function refreshJuryBox() {
  const box = document.getElementById("jury-box");
  if (!box) return;
  let total = 0n;
  for (const el of list.querySelectorAll("[data-jury-tally]")) {
    total += BigInt(el.dataset.agent || 0) + BigInt(el.dataset.shadow || 0);
  }
  box.innerHTML = `<strong>${total}</strong> juror verdict${total === 1n ? "" : "s"} on-chain across this queue` +
    `<div class="jury-roster" id="jury-roster"></div>`;
  refreshRoster();
}

// The server reconstructs the roster from JurorVerdict logs and labels
// wallets via Nansen when configured — so a "Smart Money" juror reads
// differently from an anonymous one. Absent labels, render addresses alone.
async function refreshRoster() {
  const roster = document.getElementById("jury-roster");
  if (!roster) return;
  try {
    const res = await fetch("/api/jury", { credentials: "omit" });
    if (!res.ok) return;
    const data = await res.json();
    if (!data.jurors || !data.jurors.length) return;
    roster.innerHTML = "";
    for (const j of data.jurors) {
      const chip = document.createElement("span");
      chip.className = "juror-chip";
      chip.textContent = shortAddr(j.address);
      const side = j.agent_votes + j.shadow_votes;
      chip.title = `${side} verdict${side === 1 ? "" : "s"} · agent ${j.agent_votes} · shadow ${j.shadow_votes}`;
      for (const l of (j.labels || []).slice(0, 3)) {
        const tag = document.createElement("em");
        tag.className = "wallet-label";
        tag.textContent = l.label;
        chip.append(" ", tag);
      }
      roster.append(chip);
    }
    if (data.jurors.some(j => j.labels_source)) {
      const credit = document.createElement("span");
      credit.className = "labels-src";
      credit.textContent = "labels by Nansen";
      roster.append(credit);
    }
  } catch { /* roster is enrichment — silent */ }
}

// Reflect the visitor's existing votes passively (eth_accounts, no popup).
async function refreshOwnVerdicts() {
  const wallet = window.ethereum;
  if (!wallet) return;
  let me;
  try { [me] = await wallet.request({ method: "eth_accounts" }); } catch { return; }
  if (!me) return;
  await Promise.all(Array.from(list.querySelectorAll(".jury-row")).map(async row => {
    try {
      const raw = await rpc("eth_call", [{ to: config.contract, data: VERDICT_OF_SELECTOR + pad32(row.dataset.juryKey) + pad32(me) }, "latest"]);
      const mine = (raw && raw !== "0x") ? Number(BigInt(raw)) : 0;
      row.querySelectorAll("[data-jury-vote]").forEach(b =>
        b.classList.toggle("juror-voted", Number(b.dataset.juryVote) === mine));
      const you = row.querySelector("[data-jury-you]");
      if (mine) you.textContent = `you: ${mine === 1 ? "agent" : "shadow"} · ${shortAddr(me)}`;
    } catch { /* leave row as-is */ }
  }));
}

async function ensureChain(wallet) {
  const want = CHAIN_PARAMS(config.chainId);
  try {
    await wallet.request({ method: "wallet_switchEthereumChain", params: [{ chainId: want.chainId }] });
  } catch {
    await wallet.request({ method: "wallet_addEthereumChain", params: [want] });
    await wallet.request({ method: "wallet_switchEthereumChain", params: [{ chainId: want.chainId }] });
  }
  const actual = await wallet.request({ method: "eth_chainId" });
  if (BigInt(actual) !== BigInt(want.chainId)) throw new Error("Wallet is on the wrong chain.");
}

async function vote(button) {
  const wallet = window.ethereum;
  const row = button.closest("[data-jury-key]");
  const status = row.querySelector("[data-jury-status]");
  if (!wallet) { status.textContent = "No wallet found — install MetaMask to join the jury."; return; }
  const buttons = row.querySelectorAll("[data-jury-vote]");
  buttons.forEach(b => (b.disabled = true));
  try {
    status.textContent = "Connect your wallet…";
    const accounts = await wallet.request({ method: "eth_requestAccounts" });
    if (!accounts.length) throw new Error("No wallet account selected.");
    await ensureChain(wallet);
    status.textContent = "Confirm the verdict in your wallet. It costs a little gas.";
    const calldata = ATTEST_SELECTOR + pad32(row.dataset.juryKey) + pad32(button.dataset.juryVote);
    const tx = await wallet.request({
      method: "eth_sendTransaction",
      params: [{ from: accounts[0], to: config.contract, data: calldata }],
    });
    status.innerHTML = "";
    const link = document.createElement("a");
    link.href = `${config.explorer}/tx/${tx}`; link.target = "_blank"; link.rel = "noopener";
    link.textContent = `${shortAddr(accounts[0])} on the jury ↗`;
    status.append(link);
    setTimeout(() => { refreshTally(row); refreshJuryBox(); refreshOwnVerdicts(); }, 2500);
  } catch (error) {
    status.textContent = error?.code === 4001 ? "Verdict cancelled." : `Jury verdict failed: ${error.message}`;
  } finally {
    buttons.forEach(b => (b.disabled = false));
  }
}

async function initRows() {
  await Promise.all(Array.from(list.querySelectorAll(".jury-row")).map(refreshTally));
  refreshJuryBox();
}

document.addEventListener("click", event => {
  const button = event.target.closest("[data-jury-vote]");
  if (button) vote(button);
});
// htmx swaps a fresh row after the human verdict — re-read its tally.
document.body.addEventListener("htmx:afterSwap", initRows);
window.ethereum?.on?.("accountsChanged", refreshOwnVerdicts);
initRows();
refreshJuryBox();
refreshOwnVerdicts();
