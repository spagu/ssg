// Stands in for "cloudflare:sockets" (see resolve.mjs). A test file that wants
// SMTP through the real import path sets globalThis.fakeSocketsConnect before
// the first send. Without it the import fails, as it does outside Workers —
// each test file runs in its own process, so both cases get exercised.
if (typeof globalThis.fakeSocketsConnect !== "function") {
  throw new Error("cloudflare:sockets is only available on Cloudflare");
}

export const connect = (...args) => globalThis.fakeSocketsConnect(...args);
