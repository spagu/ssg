// RFC 8291 and RFC 8292. Run from the repository root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { encryptPayload, importPrivate, sendPush, vapidAuthorization } from "../functions/api/push/_webpush.ts";
import { b64urlDecode, b64urlEncode } from "../functions/api/push/_lib.ts";

const dec = (s: string) => b64urlDecode(s) as Uint8Array;

// RFC 8291, Appendix A.
const RFC = {
  plaintext: "When I grow up, I want to be a watermelon",
  asPrivate: "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
  asPublic: "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8",
  uaPublic: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
  auth: "BTBZMqHH6r4Tts7J_aSIgg",
  salt: "DGv6ra1nlYgDCS1FRnbzlw",
  body:
    "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN",
};

test("encryptPayload reproduces the RFC 8291 example", async () => {
  const sender = { privateKey: await importPrivate(dec(RFC.asPrivate), dec(RFC.asPublic), "ecdh"), publicKey: dec(RFC.asPublic) };
  const out = await encryptPayload(
    new TextEncoder().encode(RFC.plaintext),
    { endpoint: "https://fcm.googleapis.com/x", p256dh: RFC.uaPublic, auth: RFC.auth },
    { salt: dec(RFC.salt), sender },
  );
  assert.equal(b64urlEncode(out), RFC.body);
});

test("encryptPayload uses a fresh salt and key each time", async () => {
  const sub = { endpoint: "https://fcm.googleapis.com/x", p256dh: RFC.uaPublic, auth: RFC.auth };
  const a = await encryptPayload(new Uint8Array([1]), sub);
  const b = await encryptPayload(new Uint8Array([1]), sub);
  assert.equal(a.length, 21 + 65 + 1 + 1 + 16);
  assert.notDeepEqual(a.slice(0, 16), b.slice(0, 16));
});

test("encryptPayload refuses malformed keys", async () => {
  await assert.rejects(encryptPayload(new Uint8Array(1), { endpoint: "https://x", p256dh: "AAAA", auth: RFC.auth }));
  await assert.rejects(encryptPayload(new Uint8Array(1), { endpoint: "https://x", p256dh: RFC.uaPublic, auth: "AAAA" }));
});

async function keyPair() {
  const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
  const pub = new Uint8Array((await crypto.subtle.exportKey("raw", pair.publicKey)) as ArrayBuffer);
  const jwk = await crypto.subtle.exportKey("jwk", pair.privateKey);
  return { verify: pair.publicKey, publicKey: b64urlEncode(pub), privateKey: jwk.d as string };
}

test("vapidAuthorization signs the RFC 8292 claims", async () => {
  const k = await keyPair();
  const now = Date.UTC(2026, 0, 1);
  const header = await vapidAuthorization("https://fcm.googleapis.com/fcm/send/abc", "mailto:news@example.com", k.publicKey, k.privateKey, now);
  const m = header.match(/^vapid t=([^.]+)\.([^.]+)\.([^,]+), k=(.+)$/);
  assert.ok(m, header);
  const [, h, c, s, key] = m;
  assert.equal(key, k.publicKey);
  assert.deepEqual(JSON.parse(new TextDecoder().decode(dec(h))), { typ: "JWT", alg: "ES256" });
  assert.deepEqual(JSON.parse(new TextDecoder().decode(dec(c))), {
    aud: "https://fcm.googleapis.com",
    exp: now / 1000 + 12 * 3600,
    sub: "mailto:news@example.com",
  });
  const ok = await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, k.verify, dec(s), new TextEncoder().encode(`${h}.${c}`));
  assert.ok(ok);
});

test("vapidAuthorization refuses malformed keys", async () => {
  await assert.rejects(vapidAuthorization("https://fcm.googleapis.com/x", "mailto:a@example.com", "AAAA", "AAAA"));
});

test("sendPush posts an encrypted, authorised message", async (t) => {
  const k = await keyPair();
  const seen: { url: string; init: RequestInit }[] = [];
  t.mock.method(globalThis, "fetch", async (url: string, init: RequestInit) => {
    seen.push({ url, init });
    return new Response(null, { status: 201 });
  });
  const status = await sendPush(
    { endpoint: "https://fcm.googleapis.com/fcm/send/abc", p256dh: RFC.uaPublic, auth: RFC.auth },
    { title: "t" },
    { subject: "mailto:news@example.com", publicKey: k.publicKey, privateKey: k.privateKey, ttl: 60 },
  );
  assert.equal(status, 201);
  const h = seen[0].init.headers as Record<string, string>;
  assert.equal(h["content-encoding"], "aes128gcm");
  assert.equal(h.ttl, "60");
  assert.match(h.authorization, /^vapid t=/);
});
