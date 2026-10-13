// Inject a real EIP-1193 provider backed by the signing proxy (attester key).
window.ethereum = {
  isMetaMask: true,
  selectedAddress: "0xE18EA17E7bEdf603a482013F59608446e95bB56c",
  request: async ({ method, params }) => {
    const r = await fetch("http://127.0.0.1:9911/", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ method, params }),
    });
    const j = await r.json();
    if (j.error) throw new Error(j.error);
    return j.result;
  },
  on: () => {},
  removeListener: () => {},
};
"injected: " + window.ethereum.selectedAddress;
