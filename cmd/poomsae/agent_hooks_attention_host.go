package main

// Shared by generated native adapters so a nested host keeps its parent's
// attention ownership unless it explicitly tracks a different issue.
const nativeAttentionHostGuardJS = `
function poomsaeAttentionHostSkip(identity) {
  try {
    const marker = JSON.parse(process.env.POOMSAE_ATTENTION_HOST || "null");
    if (!marker || !Number.isInteger(marker.pid) || marker.pid <= 0 ||
        marker.pid === process.pid || typeof marker.identity !== "string" ||
        !marker.identity || marker.identity !== identity) return false;
    try { process.kill(marker.pid, 0); return true; }
    catch (error) { return error.code === "EPERM"; }
  } catch { return false; }
}
function poomsaeAttentionHostClaim(identity) {
  process.env.POOMSAE_ATTENTION_HOST = JSON.stringify({pid: process.pid, identity});
}
function poomsaeAttentionHostRelease(identity) {
  if (process.env.POOMSAE_ATTENTION_HOST === JSON.stringify({pid: process.pid, identity})) {
    delete process.env.POOMSAE_ATTENTION_HOST;
  }
}
`
