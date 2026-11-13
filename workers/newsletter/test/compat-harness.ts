// The machinery behind the backward-compatibility suite (compat-*.test.ts):
// a frozen clock, deterministic random bytes, a recording fetch, and one
// `call` that runs a Pages handler the way Pages does and records everything a
// client or a mail provider could observe. The same scenarios were run against
// the 1.9.7 release of this template and the results frozen in
// compat-golden.json; the suite runs them against the current code.
import { createHash } from "node:crypto";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";
import type { SqliteD1 } from "./sqlite-d1.ts";

type Fn = (ctx: { request: Request; env: unknown; waitUntil: (p: Promise<unknown>) => void }) => Promise<Response>;
export interface Handlers {
  signup: { onRequestPost: Fn };
  confirm: { onRequestGet: Fn; onRequestPost: Fn };
  unsubscribe: { onRequestGet: Fn; onRequestPost: Fn };
  admin: { onRequestGet: Fn; onRequestPost: Fn };
}

// loadHandlers imports the four endpoints from a functions/api/newsletter
// directory (a URL ending in "/"), so the scenarios can run on any copy.
export async function loadHandlers(dir: string): Promise<Handlers> {
  const [signup, confirm, unsubscribe, admin] = await Promise.all(
    ["index.ts", "confirm.ts", "unsubscribe.ts", "admin.ts"].map((f) => import(new URL(f, dir).href)),
  );
  return { signup, confirm, unsubscribe, admin };
}

export const T0 = Date.parse("2026-03-01T10:00:00.000Z");
export const ADMIN = "Basic " + btoa("admin:admin-pass");

// Observed is one call as a client and the providers saw it. An HTML page is
// kept as its hash plus its visible heading and paragraph, which is what a
// failure message needs; JSON, CSV and plain text are kept whole.
export interface Observed {
  step: string;
  status: number;
  headers: [string, string][];
  body: string;
  fetches: { url: string; method: string; headers: [string, string][]; body: string }[];
}

const sha = (s: string) => createHash("sha256").update(s).digest("hex");

function summarise(type: string, text: string): string {
  if (!type.startsWith("text/html")) return text;
  const h1 = /<h1>([^<]*)<\/h1>/.exec(text)?.[1] ?? "";
  const p = /<main>[\s\S]*?<p>([^<]*)<\/p>/.exec(text)?.[1] ?? "";
  const form = /<form [^>]*action="([^"]*)"/.exec(text)?.[1];
  return `html sha256=${sha(text)} h1=${h1} p=${p}${form ? ` form=${form}` : ""}`;
}

async function bodyOf(init: RequestInit | undefined): Promise<string> {
  const b = init?.body;
  if (b == null) return "";
  if (typeof b === "string") return b;
  if (b instanceof FormData) return JSON.stringify([...b.entries()].map(([k, v]) => [k, String(v)]));
  return String(b);
}

export interface World {
  db: SqliteD1;
  env: Record<string, unknown>;
  log: Observed[];
  logs: string[];
  now: number;
  providerStatus: number; // what the mail providers answer
  providerThrows: boolean;
  call(handler: Fn, step: string, init: { method?: string; path: string; headers?: Record<string, string>; body?: string; env?: Record<string, unknown> }): Promise<Observed>;
  tokenOf(email: string): string;
  rows(): Record<string, unknown>[];
  advance(ms: number): void;
}

// withWorld runs fn with the clock, random source, fetch and console replaced
// and a fresh v1 database, and always puts the globals back.
export async function withWorld<T>(fn: (w: World) => Promise<T>): Promise<T> {
  const RealDate = Date;
  const realFetch = globalThis.fetch;
  const realRandom = crypto.getRandomValues;
  const realUUID = crypto.randomUUID;
  const real = { warn: console.warn, error: console.error };
  let seed = 0;
  let uuid = 0;

  const w: World = {
    db: withSchemaV1(sqliteD1()),
    env: {},
    log: [],
    logs: [],
    now: T0,
    providerStatus: 200,
    providerThrows: false,
    async call(handler, step, init) {
      const fetches: Observed["fetches"] = [];
      globalThis.fetch = (async (input: string | URL, ri?: RequestInit) => {
        const url = String(input);
        fetches.push({ url, method: ri?.method ?? "GET", headers: Object.entries((ri?.headers ?? {}) as Record<string, string>), body: await bodyOf(ri) });
        // A Turnstile token "bad" is refused, any other passes.
        if (url.includes("turnstile")) return Response.json({ success: !fetches[fetches.length - 1].body.includes('["response","bad"]') });
        if (w.providerThrows) throw new Error("network down");
        return new Response('{"id":"msg-1"}', { status: w.providerStatus });
      }) as typeof fetch;
      const pending: Promise<unknown>[] = [];
      const request = new Request(`https://example.com${init.path}`, { method: init.method ?? "GET", headers: init.headers, body: init.body });
      const env = { ...w.env, ...init.env };
      const res = await handler({ request, env, waitUntil: (p) => void pending.push(p) });
      await Promise.all(pending);
      // ignoreBOM keeps a byte-order mark in the text (Response.text() drops it).
      const text = new TextDecoder("utf-8", { ignoreBOM: true }).decode(await res.arrayBuffer());
      const headers = [...res.headers.entries()].sort();
      const o: Observed = { step, status: res.status, headers, body: summarise(res.headers.get("content-type") ?? "", text), fetches };
      w.log.push(o);
      return o;
    },
    tokenOf(email) {
      return String((w.db.sqlite.prepare("SELECT token FROM subscribers WHERE email = ?").get(email) as { token: string }).token);
    },
    rows() {
      return w.db.sqlite.prepare("SELECT * FROM subscribers ORDER BY email").all().map((r) => ({ ...r }));
    },
    advance(ms) {
      w.now += ms;
    },
  };
  w.env = { NEWSLETTER_DB: w.db, NEWSLETTER_TURNSTILE_SECRET: "ts-secret", NEWSLETTER_ADMIN_PASSWORD: "admin-pass" };

  class FrozenDate extends RealDate {
    constructor(...a: unknown[]) {
      if (a.length) super(...(a as [string]));
      else super(w.now);
    }
    static now() {
      return w.now;
    }
  }
  globalThis.Date = FrozenDate as DateConstructor;
  crypto.getRandomValues = (<T extends ArrayBufferView | null>(a: T): T => {
    const bytes = new Uint8Array((a as ArrayBufferView).buffer, (a as ArrayBufferView).byteOffset, (a as ArrayBufferView).byteLength);
    for (let i = 0; i < bytes.length; i++) bytes[i] = (seed++ * 37 + 11) & 0xff;
    return a;
  }) as typeof crypto.getRandomValues;
  crypto.randomUUID = (() => `00000000-0000-4000-8000-${String(++uuid).padStart(12, "0")}`) as typeof crypto.randomUUID;
  console.warn = (...a: unknown[]) => void w.logs.push("warn: " + a.join(" "));
  console.error = (...a: unknown[]) => void w.logs.push("error: " + a.join(" "));
  try {
    return await fn(w);
  } finally {
    globalThis.Date = RealDate;
    globalThis.fetch = realFetch;
    crypto.getRandomValues = realRandom;
    crypto.randomUUID = realUUID;
    console.warn = real.warn;
    console.error = real.error;
  }
}
