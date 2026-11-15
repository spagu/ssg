// The small helpers. Several of them are the only thing standing between a
// buyer's text and somewhere that text must not reach.

import { env } from "./_env";
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  base64url,
  base64urlToBytes,
  cached,
  csvCell,
  escapeHtml,
  fail,
  ipHash,
  isValidEmail,
  json,
  newId,
  normaliseVatId,
  nowISO,
  readBody,
  readJson,
  requestCountry,
  safeFilename,
  sha256hex,
  str,
  timingSafeEqual,
  userAgent,
  verifyTurnstile,
} from "../functions/api/shop/_lib";

describe("responses", () => {
  it("never lets a browser sniff or cache an API answer", async () => {
    const res = json({ ok: true });
    assert.equal(res.headers.get("cache-control"), "no-store");
    assert.equal(res.headers.get("x-content-type-options"), "nosniff");
    assert.equal(res.headers.get("referrer-policy"), "no-referrer");
    assert.deepEqual(await res.json(), { ok: true });
  });

  it("marks the two public reads as cacheable", () => {
    assert.equal(cached({}, 60).headers.get("cache-control"), "public, max-age=60");
  });

  it("carries the Cloudflare ray into an error so support can find the log", async () => {
    const request = new Request("https://x.example/a", { headers: { "cf-ray": "abc123" } });
    const body = (await fail(request, "nope", "Nope.", 422).json()) as Record<string, string>;
    assert.deepEqual(body, { error: "nope", message: "Nope.", requestId: "abc123" });
  });

  it("omits the ray when there is none", async () => {
    const body = (await fail(new Request("https://x.example/a"), "nope", "Nope.", 400).json()) as object;
    assert.ok(!("requestId" in (body)));
  });
});

describe("ids, hashes and tokens", () => {
  it("makes distinct ids", () => {
    assert.notEqual(newId(), newId());
  });

  it("hashes text and bytes the same way", async () => {
    const fromText = await sha256hex("abc");
    const fromBytes = await sha256hex(new TextEncoder().encode("abc").buffer as ArrayBuffer);
    assert.equal(fromText, fromBytes);
    assert.equal(fromText, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  });

  it("round-trips base64url without padding", () => {
    const bytes = new Uint8Array([251, 255, 0, 1, 2]);
    const encoded = base64url(bytes);
    assert.doesNotMatch(encoded, /[+/=]/);
    assert.deepEqual([...base64urlToBytes(encoded)], [...bytes]);
  });

  it("produces an ISO timestamp", () => {
    assert.match(nowISO(), /^\d{4}-\d\d-\d\dT/);
  });
});

describe("constant-time comparison", () => {
  it("agrees with ordinary equality", () => {
    assert.equal(timingSafeEqual("secret", "secret"), true);
    assert.equal(timingSafeEqual("secret", "secrex"), false);
    assert.equal(timingSafeEqual("secret", "secretly"), false);
    assert.equal(timingSafeEqual("", ""), false); // an empty secret matches nothing
  });
});

describe("the IP hash", () => {
  it("records nothing at all without a salt", async () => {
    // An unsalted hash of an IPv4 address is reversible by brute force in
    // seconds, so it would be personal data wearing a hat.
    const request = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.9" } });
    assert.equal(await ipHash(request, { ...env, SHOP_IP_SALT: undefined }), null);
  });

  it("is stable for one address and different for another", async () => {
    const one = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.9" } });
    const two = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.10" } });
    const salted = { ...env, SHOP_IP_SALT: "pepper" };
    assert.equal(await ipHash(one, salted), await ipHash(one, salted));
    assert.notEqual(await ipHash(one, salted), await ipHash(two, salted));
  });

  it("records nothing when Cloudflare passed no address", async () => {
    assert.equal(await ipHash(new Request("https://x.example/a"), { ...env, SHOP_IP_SALT: "pepper" }), null);
  });
});

describe("reading the request", () => {
  it("takes the country Cloudflare resolved, and nothing else", () => {
    // Cloudflare attaches `cf` to the incoming Request; Node's Request has no
    // such property, so it is defined on the instance the way the runtime
    // presents it — read-only.
    const from = (country: unknown): Request =>
      Object.defineProperty(new Request("https://x.example/a"), "cf", { value: { country } });
    assert.equal(requestCountry(from("PL")), "PL");
    assert.equal(requestCountry(from("not-a-country")), null);
    assert.equal(requestCountry(from(undefined)), null);
    assert.equal(requestCountry(new Request("https://x.example/a")), null);
  });

  it("caps the user agent", () => {
    const long = "x".repeat(900);
    const request = new Request("https://x.example/a", { headers: { "user-agent": long } });
    assert.equal(userAgent(request).length, 512);
    assert.equal(userAgent(new Request("https://x.example/a")), "");
  });

  it("reads JSON, and says null rather than throwing on rubbish", async () => {
    const good = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: '{"a":1}',
    });
    assert.deepEqual(await readJson(good), { a: 1 });

    const bad = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{oh no",
    });
    assert.equal(await readJson(bad), null);
  });

  it("reads a form as well, so the storefront works without JavaScript", async () => {
    const form = new FormData();
    form.append("sku", "EBOOK-1");
    form.append("quantity", "2");
    const request = new Request("https://x.example/a", { method: "POST", body: form });
    assert.deepEqual(await readBody(request), { sku: "EBOOK-1", quantity: "2" });
  });

  it("falls back to JSON when the content type says nothing", async () => {
    const request = new Request("https://x.example/a", { method: "POST", body: '{"a":2}' });
    assert.deepEqual(await readBody(request), { a: 2 });
  });

  it("says null on a form body it cannot parse", async () => {
    const request = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "multipart/form-data; boundary=nope" },
      body: "not really a form",
    });
    assert.equal(await readBody(request), null);
  });
});

describe("validation", () => {
  it("trims and caps a string", () => {
    assert.equal(str("  hello  ", 20), "hello");
    assert.equal(str("abcdef", 3), "abc");
    assert.equal(str(42, 10), "");
    assert.equal(str(undefined, 10), "");
  });

  it("knows an email when it sees one", () => {
    assert.equal(isValidEmail("a@b.co"), true);
    assert.equal(isValidEmail("no-at-sign"), false);
    assert.equal(isValidEmail("a@b"), false);
    assert.equal(isValidEmail(`${"x".repeat(250)}@b.co`), false);
  });

  it("normalises a VAT number without deciding whether it is real", () => {
    assert.equal(normaliseVatId(" pl 123-456.7890 "), "PL1234567890");
  });
});

describe("escaping on the way out", () => {
  it("escapes the five characters that matter", () => {
    assert.equal(escapeHtml(`<a href="x">&'`), "&lt;a href=&quot;x&quot;&gt;&amp;&#39;");
    assert.equal(escapeHtml(null), "");
  });

  it("defuses a CSV cell that would become a formula", () => {
    // The accountant opens this in Excel; a product name starting with = is a
    // formula unless something stops it.
    assert.equal(csvCell("=1+1"), `"'=1+1"`);
    assert.equal(csvCell("+49 123"), `"'+49 123"`);
    assert.equal(csvCell("-5"), `"'-5"`);
    assert.equal(csvCell("@user"), `"'@user"`);
    assert.equal(csvCell('say "hi"'), `"say ""hi"""`);
    assert.equal(csvCell(null), `""`);
  });

  it("strips what would break a Content-Disposition header", () => {
    assert.equal(safeFilename('a"b;c\\d\ne'), "a_b_c_d_e");
    assert.equal(safeFilename(""), "download");
    assert.equal(safeFilename("x".repeat(300)).length, 200);
  });
});

describe("Turnstile", () => {
  const original = globalThis.fetch;

  it("believes a success and disbelieves everything else", async () => {
    globalThis.fetch = (async () => new Response(JSON.stringify({ success: true }))) as typeof fetch;
    assert.equal(await verifyTurnstile("secret", "token", "203.0.113.1"), true);

    globalThis.fetch = (async () => new Response(JSON.stringify({ success: false }))) as typeof fetch;
    assert.equal(await verifyTurnstile("secret", "token", null), false);

    // A network failure is not a pass: the check has to be positive.
    globalThis.fetch = (async () => {
      throw new Error("offline");
    }) as typeof fetch;
    assert.equal(await verifyTurnstile("secret", "token", null), false);

    globalThis.fetch = original;
  });
});
