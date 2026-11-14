// notify's two fallbacks: a feed served without a Content-Type is still
// parsed (by sniffing), and an item without a title is announced by its URL.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as notify } from "../functions/api/push/notify.ts";
import { b64urlEncode } from "../functions/api/push/_lib.ts";
import { ctx, fakeDB, fakeKV, SUB } from "./fakes.ts";

const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign"])) as CryptoKeyPair;

test("notify: no Content-Type on the feed, an untitled item → announced by its URL", async (t) => {
  const db = fakeDB();
  db.rows.set(SUB.endpoint, { endpoint: SUB.endpoint, p256dh: SUB.keys.p256dh, auth: SUB.keys.auth, ua_hash: null });
  const feed = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><link href="/p/2/"/><id>2</id></entry><entry><title>One</title><link href="/p/1/"/><id>1</id></entry></feed>`;
  let pushes = 0;
  t.mock.method(globalThis, "fetch", async (url: string) => {
    if (url.startsWith("https://fcm.googleapis.com/")) return (pushes++, new Response(null, { status: 201 }));
    return new Response(new TextEncoder().encode(feed)); // a byte body: no Content-Type at all
  });
  const env = {
    PUSH_DB: db,
    PUSH_KV: fakeKV({ "last-item": "1" }),
    PUSH_KEY: "k",
    PUSH_SUBJECT: "mailto:news@example.com",
    PUSH_VAPID_PUBLIC_KEY: b64urlEncode(new Uint8Array((await crypto.subtle.exportKey("raw", pair.publicKey)) as ArrayBuffer)),
    PUSH_VAPID_PRIVATE_KEY: (await crypto.subtle.exportKey("jwk", pair.privateKey)).d,
  };
  const res = await notify(ctx(new Request("https://example.com/api/push/notify", { method: "POST", headers: { authorization: "Bearer k" } }), env));
  assert.deepEqual(await res.json(), { sent: 1, failed: 0, removed: 0, items: [{ title: "", url: "https://example.com/p/2/" }] });
  assert.equal(pushes, 1);
});
