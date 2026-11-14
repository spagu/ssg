// One question to one module: "are you there?" (its health endpoint), "what
// are you?" (its manifest) or "how are you doing?" (its summary) — and, for
// the site itself, its published build report. A leading underscore keeps
// this file out of the Pages route table.
//
// The limits are what keep a broken or hostile module from hurting core:
//   - the URL comes from _target.ts (same origin as the caller's request);
//   - redirect: "manual" — a redirect is never followed, so a module cannot
//     send core, with the caller's credential, anywhere else;
//   - a timeout, after which the request is aborted and the answer given;
//   - a size cap on the body that is read (a health answer is never read);
//   - the request carries the caller's own credential and nothing else: the
//     `cookie` and `authorization` headers as the caller sent them. core has
//     no key of its own to any module; a module that refuses the caller
//     refuses core asking for them.

import { checkManifest } from "./_manifests";
import type { ModuleManifest } from "./_manifests";

export type Status = "reachable" | "unreachable" | "denied" | "invalid_manifest" | "invalid" | "unknown";
export interface ProbeResult {
  status: Status;
  ms: number | null; // time to the answer; null when nothing was asked
  httpStatus?: number;
  reason?: string; // why it is not "reachable", as a snake_case word
  detail?: string; // for an invalid answer: the reader's first finding
  manifest?: ModuleManifest; // a manifest answer that passed the reader
  value?: unknown; // any other answer that passed its reader (a summary, a build report)
}

// Reader: how the body of an answer is read. `tag` keeps answers read
// differently apart in the cache; `invalid` is the status of a body the reader
// refuses (or that is too large or not JSON).
export type Read = { ok: true; found: Pick<ProbeResult, "manifest" | "value"> } | { ok: false; detail: string };
export interface Reader {
  tag: string;
  max: number;
  invalid: Status;
  read(raw: unknown): Read;
}

// FORWARDED: the only request headers that ever leave core.
export const FORWARDED = ["cookie", "authorization"] as const;
// MAX_BYTES: a manifest at the format's limits (32 permissions, 12 nav
// entries, five 600-character texts each) stays well under this.
export const MAX_BYTES = 256 * 1024;

export function credentialHeaders(request: Request): Headers {
  const out = new Headers();
  for (const name of FORWARDED) {
    const value = request.headers.get(name);
    if (value !== null) out.set(name, value);
  }
  return out;
}

// discard lets go of a body core will not read. Not awaited: cancelling must
// never be what an answer waits for.
function discard(res: Response): void {
  void res.body?.cancel().catch(() => undefined);
}

// readCapped returns the body as text, or null when it is larger than max —
// by its declared length, or by what actually arrives.
export async function readCapped(res: Response, max: number): Promise<string | null> {
  if (Number(res.headers.get("content-length")) > max) {
    discard(res);
    return null;
  }
  const reader = res.body!.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > max) {
      void reader.cancel().catch(() => undefined);
      return null;
    }
    chunks.push(value);
  }
  const all = new Uint8Array(total);
  let at = 0;
  for (const c of chunks) {
    all.set(c, at);
    at += c.byteLength;
  }
  return new TextDecoder().decode(all);
}

// manifestReader accepts a manifest only when it describes module `id`.
export function manifestReader(id: string): Reader {
  return {
    tag: `manifest:${id}`,
    max: MAX_BYTES,
    invalid: "invalid_manifest",
    read(raw) {
      const checked = checkManifest(raw, id);
      return checked.ok ? { ok: true, found: { manifest: checked.manifest } } : { ok: false, detail: checked.detail };
    },
  };
}

// classify turns an answer into a result; ms is already measured.
async function classify(res: Response, ms: number, reader: Reader | null): Promise<ProbeResult> {
  const httpStatus = res.status;
  const done = (r: ProbeResult) => {
    discard(res);
    return r;
  };
  // With redirect: "manual" a Worker gets the 3xx itself, never the target.
  if (httpStatus >= 300 && httpStatus < 400) return done({ status: "unreachable", ms, httpStatus, reason: "redirect" });
  if (httpStatus === 401 || httpStatus === 403) return done({ status: "denied", ms, httpStatus });
  if (!res.ok) return done({ status: "unreachable", ms, httpStatus, reason: "http_status" });
  if (reader === null) return done({ status: "reachable", ms, httpStatus });
  const text = await readCapped(res, reader.max);
  if (text === null) return { status: reader.invalid, ms, httpStatus, reason: "too_large" };
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { status: reader.invalid, ms, httpStatus, reason: "not_json" };
  }
  const read = reader.read(raw);
  if (!read.ok) return { status: reader.invalid, ms, httpStatus, reason: "rejected", detail: read.detail };
  return { status: "reachable", ms, httpStatus, ...read.found };
}

// probe asks url, giving up after timeoutMs (then the result is unreachable
// with `timeoutReason`). reader: how the body is read, or null for a health
// check whose body is not read at all.
export async function probe(url: URL, request: Request, timeoutMs: number, reader: Reader | null, timeoutReason = "timeout"): Promise<ProbeResult> {
  const started = Date.now();
  const controller = new AbortController();
  let timer!: ReturnType<typeof setTimeout>; // set synchronously by the executor below
  const timedOut = new Promise<ProbeResult>((resolve) => {
    timer = setTimeout(() => {
      controller.abort();
      resolve({ status: "unreachable", ms: timeoutMs, reason: timeoutReason });
    }, timeoutMs);
  });
  const asked = (async (): Promise<ProbeResult> => {
    try {
      const res = await fetch(url.href, { method: "GET", headers: credentialHeaders(request), redirect: "manual", signal: controller.signal });
      return await classify(res, Date.now() - started, reader);
    } catch {
      return { status: "unreachable", ms: Date.now() - started, reason: "network" };
    }
  })();
  try {
    return await Promise.race([asked, timedOut]);
  } finally {
    clearTimeout(timer);
  }
}
