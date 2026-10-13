// Permissionless juror verdicts: the visitor's own wallet sends
// attest(bytes32,uint8) to SekatsJury — no relay, no server key. Tallies
// are read back through the public RPC, never trusted from the client.
const list = document.getElementById("review-list");
const config = {
  contract: list.dataset.juryContract,
  rpc: list.dataset.juryRpc,
  chainId: Number(list.dataset.juryChain),
  explorer: list.dataset.juryExplorer,
};
const ATTEST_SELECTOR = "0xd1143450"; // attest(bytes32,uint8)
const TALLY_SELECTOR = "0xf6e22eb5"; // tally(bytes32)
const pad32 = hex => hex.replace(/^0x/, "").padStart(64, "0");

const CHAIN_PARAMS = id => ({
  chainId: "0x" + id.toString(16),
  chainName: "Monad",
  nativeCurrency: { name: "MON", symbol: "MON", decimals: 18 },
  rpcUrls: [config.rpc],
  blockExplorerUrls: [config.explorer],
});

async function rpcCall(data) {
  const response = await fetch(config.rpc, {
    method: "POST", headers: { "Content-Type": "application/json" }, credentials: "omit",
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "eth_call", params: [{ to: config.contract, data }, "latest"] }),
  });
  if (!response.ok) throw new Error("RPC unavailable");
  const result = await response.json();
  if (result.error || typeof result.result !== "string") throw new Error("RPC response error");
  return result.result;
}

async function refreshTally(row) {
  const el = row.querySelector("[data-jury-tally]");
  try {
    const raw = await rpcCall(TALLY_SELECTOR + pad32(row.dataset.juryKey));
    const agent = BigInt("0x" + raw.slice(2, 66));
    const shadow = BigInt("0x" + raw.slice(66, 130));
    el.textContent = `agent ${agent} · shadow ${shadow}`;
  } catch {
    el.textContent = "tally unavailable";
  }
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
    link.textContent = "verdict recorded on-chain ↗";
    status.append(link);
    // Give the RPC a moment to index before re-reading the tally.
    setTimeout(() => refreshTally(row), 2500);
  } catch (error) {
    status.textContent = error?.code === 4001 ? "Verdict cancelled." : `Jury verdict failed: ${error.message}`;
  } finally {
    buttons.forEach(b => (b.disabled = false));
  }
}

function initRows() {
  for (const row of list.querySelectorAll(".jury-row")) refreshTally(row);
}

document.addEventListener("click", event => {
  const button = event.target.closest("[data-jury-vote]");
  if (button) vote(button);
});
// htmx swaps a fresh row after the human verdict — re-read its tally.
document.body.addEventListener("htmx:afterSwap", initRows);
initRows();
