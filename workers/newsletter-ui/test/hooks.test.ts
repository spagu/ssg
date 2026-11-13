// GO-127: the provider webhooks, through the whole pipeline (no auth, no
// CSRF, no rate limit) — reachable by anyone, acted on only when signed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_BODY } from "../src/hook-routes.ts";
import { findSuppression } from "../src/suppress.ts";
import { fakeRateLimiter } from "./fakes.ts";
import { MC_PEM, RESEND_SECRET, hook, hookEnv, mcEvent, resendEvent, rfc9421Headers, svixHeaders } from "./hook-fixtures.ts";
import { row, seed } from "./subscriber-seed.ts";

type Env = Awaited<ReturnType<typeof hookEnv>>;
const count = (env: Env, table: string) => Number((env.NEWSLETTER_DB.sqlite.prepare(`SELECT COUNT(*) AS n FROM ${table}`).get() as { n: number }).n);
const written = (env: Env) => count(env, "suppressions") + count(env, "soft_bounces") + count(env, "hook_events");

test("Resend: a valid signature is accepted without auth, CSRF or the per-IP limit", async () => {
  const env = await hookEnv({ RATE_LIMITER: fakeRateLimiter(0) });
  const body = resendEvent("email.bounced", "gone@example.com", { type: "Permanent", subType: "General" });
  const res = await hook(env, "resend", body, await svixHeaders(body));
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { ok: true, applied: 1, duplicate: 0, ignored: 0 });
  assert.equal(res.headers.get("cache-control"), "no-store");
  const s = await findSuppression(env as never, "gone@example.com");
  assert.equal(s?.reason, "hard");
  assert.equal(s?.source, "resend");
  assert.equal(s?.email, "gone@example.com");
});

test("Resend: forged, missing or stale signatures → 401 and nothing written", async () => {
  const env = await hookEnv();
  const body = resendEvent("email.bounced", "a@example.com");
  const good = await svixHeaders(body);
  const variants: [string, Record<string, string>, string?][] = [
    ["forged with another secret", await svixHeaders(body, { secret: "whsec_" + btoa("another secret, same length!") })],
    ["body changed after signing", good, resendEvent("email.bounced", "b@example.com")],
    ["no svix-signature", { ...good, "svix-signature": "" }],
    ["no svix-id", { ...good, "svix-id": "" }],
    ["no svix-timestamp", { ...good, "svix-timestamp": "" }],
    ["id changed after signing", { ...good, "svix-id": "msg_other" }],
    ["stale timestamp", await svixHeaders(body, { ts: Math.floor(Date.now() / 1000) - 301 })],
    ["future timestamp", await svixHeaders(body, { ts: Math.floor(Date.now() / 1000) + 301 })],
    ["timestamp not a number", { ...good, "svix-timestamp": "soon" }],
    ["wrong version tag", { ...good, "svix-signature": good["svix-signature"].replace("v1,", "v2,") }],
    ["signature not base64", { ...good, "svix-signature": "v1,%%%" }],
  ];
  for (const [name, headers, sent] of variants) {
    const res = await hook(env, "resend", sent ?? body, headers);
    assert.equal(res.status, 401, name);
    assert.equal((await res.json()).error, "invalid_signature", name);
  }
  assert.equal(written(env), 0);
  // Several signatures (a secret rotation): one valid one is enough.
  const rotated = { ...good, "svix-signature": `v1,AAAA ${good["svix-signature"]}` };
  assert.equal((await hook(env, "resend", body, rotated)).status, 200);
});

test("Resend: unset or unusable secret → 503, never open", async () => {
  const body = resendEvent("email.bounced", "a@example.com");
  const unset = await hookEnv({ NEWSLETTER_RESEND_WEBHOOK_SECRET: "" });
  const res = await hook(unset, "resend", body, await svixHeaders(body));
  assert.equal(res.status, 503);
  assert.equal((await res.json()).error, "hook_not_configured");
  const broken = await hookEnv({ NEWSLETTER_RESEND_WEBHOOK_SECRET: "whsec_%%%" });
  assert.equal((await hook(broken, "resend", body, await svixHeaders(body))).status, 401);
  const empty = await hookEnv({ NEWSLETTER_RESEND_WEBHOOK_SECRET: "whsec_" });
  assert.equal((await hook(empty, "resend", body, await svixHeaders(body))).status, 401);
  assert.equal(written(unset) + written(broken) + written(empty), 0);
});

test("Resend: a replayed event id counts once", async () => {
  const env = await hookEnv();
  const body = resendEvent("email.bounced", "a@example.com", { type: "Transient" });
  const id = "msg_replayed";
  const first = await (await hook(env, "resend", body, await svixHeaders(body, { id }))).json();
  // A provider retry is signed again (new timestamp), same id.
  const again = await (await hook(env, "resend", body, await svixHeaders(body, { id, ts: Math.floor(Date.now() / 1000) - 10 }))).json();
  assert.deepEqual([first.applied, again.applied, again.duplicate], [1, 0, 1]);
  assert.equal((env.NEWSLETTER_DB.sqlite.prepare("SELECT count FROM soft_bounces").get() as { count: number }).count, 1);
});

test("Resend: a complaint suppresses and unsubscribes; delivered and others", async () => {
  const env = await hookEnv();
  const [id] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com" }]);
  const body = resendEvent("email.complained", "ann@example.com");
  assert.equal((await hook(env, "resend", body, await svixHeaders(body))).status, 200);
  assert.equal((await findSuppression(env as never, "ann@example.com"))?.reason, "complaint");
  const r = row(env.NEWSLETTER_DB, id) as Record<string, unknown>;
  assert.equal(r.status, "unsubscribed");
  assert.match(String(r.unsubscribed_at), /^\d{4}-/);
  assert.equal(r.updated_at, r.unsubscribed_at);

  const opened = resendEvent("email.opened", "ann@example.com");
  assert.deepEqual(await (await hook(env, "resend", opened, await svixHeaders(opened))).json(), { ok: true, applied: 0, duplicate: 0, ignored: 1 });
  const delivered = resendEvent("email.delivered", "ann@example.com");
  assert.equal((await (await hook(env, "resend", delivered, await svixHeaders(delivered))).json()).applied, 1);
  assert.equal(count(env, "hook_events"), 1, "delivered events are not recorded for dedup");

  const junk = "{not json";
  assert.equal((await hook(env, "resend", junk, await svixHeaders(junk))).status, 400);
});

test("hooks: oversized bodies → 413, other methods → 405, unknown hooks → 404", async () => {
  const env = await hookEnv();
  const big = "x".repeat(MAX_BODY + 1);
  assert.equal((await hook(env, "resend", "{}", { "content-length": String(MAX_BODY + 1) })).status, 413);
  assert.equal((await hook(env, "resend", big, await svixHeaders(big))).status, 413);
  assert.equal((await hook(env, "mailchannels", big, {})).status, 413);
  assert.equal((await hook(env, "nope", "{}", {})).status, 404);
});

test("MailChannels: a valid RFC 9421 signature, several events, each class", async () => {
  const env = await hookEnv({ RATE_LIMITER: fakeRateLimiter(0) });
  const [ann] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com" }]);
  const body = JSON.stringify([
    mcEvent("hard-bounced", ["gone@example.com"], { status: "550", reason: "5.1.1 user unknown" }),
    mcEvent("complained", ["ann@example.com"]),
    mcEvent("hard-bounced", ["policy@example.com"], { status: "554", reason: "5.7.1 spam" }),
    mcEvent("soft-bounced", ["full@example.com"], { status: "452" }),
    mcEvent("delivered", ["ok@example.com"]),
    mcEvent("processed", ["ok@example.com"]),
  ]);
  const res = await hook(env, "mailchannels", body, await rfc9421Headers(body));
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { ok: true, applied: 3, duplicate: 0, ignored: 3 });
  assert.equal((await findSuppression(env as never, "gone@example.com"))?.reason, "hard");
  assert.equal((await findSuppression(env as never, "ann@example.com"))?.reason, "complaint");
  assert.equal(row(env.NEWSLETTER_DB, ann)?.status, "unsubscribed");
  assert.equal(await findSuppression(env as never, "policy@example.com"), null, "a policy refusal never suppresses");
  assert.equal(count(env, "soft_bounces"), 0, "MailChannels soft-bounced is not counted");

  // The same request again: every event is a duplicate.
  const again = await (await hook(env, "mailchannels", body, await rfc9421Headers(body))).json();
  assert.deepEqual([again.applied, again.duplicate], [1, 2], "only the delivered reset runs again");
  // A single object instead of an array is read as one event.
  const one = JSON.stringify(mcEvent("hard-bounced", ["x@example.com"], { smtp_id: "<m2@example.com>" }));
  assert.equal((await (await hook(env, "mailchannels", one, await rfc9421Headers(one))).json()).applied, 1);
  const junk = "[";
  assert.equal((await hook(env, "mailchannels", junk, await rfc9421Headers(junk))).status, 400);
});

test("MailChannels: forged, missing or stale signatures → 401 and nothing written", async () => {
  const env = await hookEnv();
  const body = JSON.stringify([mcEvent("hard-bounced", ["a@example.com"], { status: "550" })]);
  const good = await rfc9421Headers(body);
  const other = (await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"])) as CryptoKeyPair;
  const old = Math.floor(Date.now() / 1000) - 301;
  const variants: [string, Record<string, string>][] = [
    ["signed by another key", await rfc9421Headers(body, { key: other.privateKey })],
    ["digest of another body", await rfc9421Headers(body, { digestBody: "[]" })],
    ["no signature", { ...good, signature: "" }],
    ["no signature-input", { ...good, "signature-input": "" }],
    ["no content-digest", { ...good, "content-digest": "" }],
    ["digest not sha-256", { ...good, "content-digest": good["content-digest"].replace("sha-256", "sha-512") }],
    ["stale created", await rfc9421Headers(body, { created: old })],
    ["created changed after signing", { ...good, "signature-input": good["signature-input"].replace(/created=\d+/, "created=" + (old + 300)) }],
    ["content-digest not covered", await rfc9421Headers(body, { components: '"content-type"' })],
    ["derived component", await rfc9421Headers(body, { components: '"content-digest" "@method"' })],
    ["another algorithm", await rfc9421Headers(body, { alg: "hmac-sha256" })],
    ["label mismatch", { ...good, signature: good.signature.replace("sig_1738775282", "sig_other") }],
    ["signature not base64", { ...good, signature: "sig_1738775282=:%%%:" }],
    ["signature not decodable", { ...good, signature: "sig_1738775282=:abc:" }],
  ];
  for (const [name, headers] of variants) assert.equal((await hook(env, "mailchannels", body, headers)).status, 401, name);
  // A covered header the request does not carry.
  const extra = await rfc9421Headers(body, { components: '"content-digest" "x-missing"' });
  assert.equal((await hook(env, "mailchannels", body, extra)).status, 401);
  assert.equal(written(env), 0);
  // Labels with characters that are special in a regular expression work.
  assert.equal((await hook(env, "mailchannels", body, await rfc9421Headers(body, { label: "sig.1-a" }))).status, 200);
});

test("MailChannels: unset or unusable key → 503; raw and bare-base64 keys work", async () => {
  const body = "[]";
  for (const key of ["", "not a key", "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----", "%%%"]) {
    const env = await hookEnv({ NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY: key });
    const res = await hook(env, "mailchannels", body, await rfc9421Headers(body));
    assert.equal(res.status, 503, JSON.stringify(key));
  }
  const { MC_KEYS } = await import("./hook-fixtures.ts");
  const raw = btoa(String.fromCharCode(...new Uint8Array(await crypto.subtle.exportKey("raw", MC_KEYS.publicKey))));
  for (const key of [raw, MC_PEM.replace(/-----[A-Z ]+-----|\s/g, "")]) {
    const env = await hookEnv({ NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY: key });
    assert.equal((await hook(env, "mailchannels", body, await rfc9421Headers(body))).status, 200);
  }
  assert.ok(RESEND_SECRET.startsWith("whsec_"));
});
