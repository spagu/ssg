// The small helpers. Several of them are the only thing standing between a
// buyer's text and somewhere that text must not reach.

import { env } from "cloudflare:test";
import { describe, expect, it } from "vitest";
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
    expect(res.headers.get("cache-control")).toBe("no-store");
    expect(res.headers.get("x-content-type-options")).toBe("nosniff");
    expect(res.headers.get("referrer-policy")).toBe("no-referrer");
    expect(await res.json()).toEqual({ ok: true });
  });

  it("marks the two public reads as cacheable", () => {
    expect(cached({}, 60).headers.get("cache-control")).toBe("public, max-age=60");
  });

  it("carries the Cloudflare ray into an error so support can find the log", async () => {
    const request = new Request("https://x.example/a", { headers: { "cf-ray": "abc123" } });
    const body = (await fail(request, "nope", "Nope.", 422).json()) as Record<string, string>;
    expect(body).toEqual({ error: "nope", message: "Nope.", requestId: "abc123" });
  });

  it("omits the ray when there is none", async () => {
    const body = (await fail(new Request("https://x.example/a"), "nope", "Nope.", 400).json()) as object;
    expect(body).not.toHaveProperty("requestId");
  });
});

describe("ids, hashes and tokens", () => {
  it("makes distinct ids", () => {
    expect(newId()).not.toBe(newId());
  });

  it("hashes text and bytes the same way", async () => {
    const fromText = await sha256hex("abc");
    const fromBytes = await sha256hex(new TextEncoder().encode("abc").buffer as ArrayBuffer);
    expect(fromText).toBe(fromBytes);
    expect(fromText).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  });

  it("round-trips base64url without padding", () => {
    const bytes = new Uint8Array([251, 255, 0, 1, 2]);
    const encoded = base64url(bytes);
    expect(encoded).not.toMatch(/[+/=]/);
    expect([...base64urlToBytes(encoded)]).toEqual([...bytes]);
  });

  it("produces an ISO timestamp", () => {
    expect(nowISO()).toMatch(/^\d{4}-\d\d-\d\dT/);
  });
});

describe("constant-time comparison", () => {
  it("agrees with ordinary equality", () => {
    expect(timingSafeEqual("secret", "secret")).toBe(true);
    expect(timingSafeEqual("secret", "secrex")).toBe(false);
    expect(timingSafeEqual("secret", "secretly")).toBe(false);
    expect(timingSafeEqual("", "")).toBe(false); // an empty secret matches nothing
  });
});

describe("the IP hash", () => {
  it("records nothing at all without a salt", async () => {
    // An unsalted hash of an IPv4 address is reversible by brute force in
    // seconds, so it would be personal data wearing a hat.
    const request = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.9" } });
    expect(await ipHash(request, { ...env, SHOP_IP_SALT: undefined })).toBe(null);
  });

  it("is stable for one address and different for another", async () => {
    const one = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.9" } });
    const two = new Request("https://x.example/a", { headers: { "cf-connecting-ip": "203.0.113.10" } });
    const salted = { ...env, SHOP_IP_SALT: "pepper" };
    expect(await ipHash(one, salted)).toBe(await ipHash(one, salted));
    expect(await ipHash(one, salted)).not.toBe(await ipHash(two, salted));
  });

  it("records nothing when Cloudflare passed no address", async () => {
    expect(await ipHash(new Request("https://x.example/a"), { ...env, SHOP_IP_SALT: "pepper" })).toBe(null);
  });
});

describe("reading the request", () => {
  it("takes the country Cloudflare resolved, and nothing else", () => {
    // `cf` is read-only on a Request inside the runtime, so it is set the one
    // way the runtime allows: through the constructor.
    const from = (country: unknown): Request =>
      new Request("https://x.example/a", { cf: { country } } as RequestInit);
    expect(requestCountry(from("PL"))).toBe("PL");
    expect(requestCountry(from("not-a-country"))).toBe(null);
    expect(requestCountry(from(undefined))).toBe(null);
    expect(requestCountry(new Request("https://x.example/a"))).toBe(null);
  });

  it("caps the user agent", () => {
    const long = "x".repeat(900);
    const request = new Request("https://x.example/a", { headers: { "user-agent": long } });
    expect(userAgent(request).length).toBe(512);
    expect(userAgent(new Request("https://x.example/a"))).toBe("");
  });

  it("reads JSON, and says null rather than throwing on rubbish", async () => {
    const good = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: '{"a":1}',
    });
    expect(await readJson(good)).toEqual({ a: 1 });

    const bad = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{oh no",
    });
    expect(await readJson(bad)).toBe(null);
  });

  it("reads a form as well, so the storefront works without JavaScript", async () => {
    const form = new FormData();
    form.append("sku", "EBOOK-1");
    form.append("quantity", "2");
    const request = new Request("https://x.example/a", { method: "POST", body: form });
    expect(await readBody(request)).toEqual({ sku: "EBOOK-1", quantity: "2" });
  });

  it("falls back to JSON when the content type says nothing", async () => {
    const request = new Request("https://x.example/a", { method: "POST", body: '{"a":2}' });
    expect(await readBody(request)).toEqual({ a: 2 });
  });

  it("says null on a form body it cannot parse", async () => {
    const request = new Request("https://x.example/a", {
      method: "POST",
      headers: { "content-type": "multipart/form-data; boundary=nope" },
      body: "not really a form",
    });
    expect(await readBody(request)).toBe(null);
  });
});

describe("validation", () => {
  it("trims and caps a string", () => {
    expect(str("  hello  ", 20)).toBe("hello");
    expect(str("abcdef", 3)).toBe("abc");
    expect(str(42, 10)).toBe("");
    expect(str(undefined, 10)).toBe("");
  });

  it("knows an email when it sees one", () => {
    expect(isValidEmail("a@b.co")).toBe(true);
    expect(isValidEmail("no-at-sign")).toBe(false);
    expect(isValidEmail("a@b")).toBe(false);
    expect(isValidEmail(`${"x".repeat(250)}@b.co`)).toBe(false);
  });

  it("normalises a VAT number without deciding whether it is real", () => {
    expect(normaliseVatId(" pl 123-456.7890 ")).toBe("PL1234567890");
  });
});

describe("escaping on the way out", () => {
  it("escapes the five characters that matter", () => {
    expect(escapeHtml(`<a href="x">&'`)).toBe("&lt;a href=&quot;x&quot;&gt;&amp;&#39;");
    expect(escapeHtml(null)).toBe("");
  });

  it("defuses a CSV cell that would become a formula", () => {
    // The accountant opens this in Excel; a product name starting with = is a
    // formula unless something stops it.
    expect(csvCell("=1+1")).toBe(`"'=1+1"`);
    expect(csvCell("+49 123")).toBe(`"'+49 123"`);
    expect(csvCell("-5")).toBe(`"'-5"`);
    expect(csvCell("@user")).toBe(`"'@user"`);
    expect(csvCell('say "hi"')).toBe(`"say ""hi"""`);
    expect(csvCell(null)).toBe(`""`);
  });

  it("strips what would break a Content-Disposition header", () => {
    expect(safeFilename('a"b;c\\d\ne')).toBe("a_b_c_d_e");
    expect(safeFilename("")).toBe("download");
    expect(safeFilename("x".repeat(300)).length).toBe(200);
  });
});

describe("Turnstile", () => {
  const original = globalThis.fetch;

  it("believes a success and disbelieves everything else", async () => {
    globalThis.fetch = (async () => new Response(JSON.stringify({ success: true }))) as typeof fetch;
    expect(await verifyTurnstile("secret", "token", "203.0.113.1")).toBe(true);

    globalThis.fetch = (async () => new Response(JSON.stringify({ success: false }))) as typeof fetch;
    expect(await verifyTurnstile("secret", "token", null)).toBe(false);

    // A network failure is not a pass: the check has to be positive.
    globalThis.fetch = (async () => {
      throw new Error("offline");
    }) as typeof fetch;
    expect(await verifyTurnstile("secret", "token", null)).toBe(false);

    globalThis.fetch = original;
  });
});
