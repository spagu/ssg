// Asking every module at once, within a deadline, and remembering the answers
// briefly. A leading underscore keeps this file out of the Pages route table.
//
// What is asked of a module:
//   - no manifest in the registry (a standalone Worker declared under
//     modules: without dir) → its manifest at {base}/api/module, the
//     convention every module follows (docs/WORKER-MODULES.md);
//   - otherwise its `health` endpoint when it declares one, else `manifest`;
//   - neither declared → "unknown", nothing is asked.
// core itself is answering this very request, so it is reachable and not asked.
// Summaries and the build report go through the same `ask` (_overview.ts), so
// every request core makes keeps the same limits and the same cache.
//
// The cache is per isolate and keyed by the URL AND a hash of the caller's
// credential: what one caller was told — "denied", or a manifest they may
// read — is never served to another caller.

import { intVar, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import type { ModuleManifest } from "./_manifests";
import { FORWARDED, manifestReader, probe } from "./_probe";
import type { ProbeResult, Reader } from "./_probe";
import type { RegistryModule } from "./_registry";
import { targetURL } from "./_target";

export const DEFAULT_MANIFEST_PATH = "/api/module";
export const CACHE_MAX_ENTRIES = 1000;

export interface Asked {
  entry: RegistryModule;
  manifest: ModuleManifest | null; // the registry's, when it passed the reader
}
export type Answer = ProbeResult & { cached?: boolean };

// Job: one request to make (a path under a base on this host, read by
// `reader`), or an answer already known without asking.
export type Job = { id: string; base: string; path: string; reader: Reader | null } | { id: string; answer: Answer };

const cache = new Map<string, { result: ProbeResult; expires: number }>();

// clearStatusCache empties the per-isolate cache (tests).
export function clearStatusCache(): void {
  cache.clear();
}

// question: which path to ask, and how its answer is read.
export function question(a: Asked): { path: string; reader: Reader | null } | null {
  if (!a.manifest) return { path: DEFAULT_MANIFEST_PATH, reader: manifestReader(a.entry.id) };
  const { health, manifest } = a.manifest.endpoints;
  if (health) return { path: health, reader: null };
  if (manifest) return { path: manifest, reader: manifestReader(a.entry.id) };
  return null;
}

async function credentialHash(request: Request): Promise<string> {
  return sha256hex(FORWARDED.map((name) => `${name}:${request.headers.get(name) ?? ""}`).join("\n"));
}

function remember(key: string, result: ProbeResult, expires: number): void {
  cache.delete(key);
  cache.set(key, { result, expires });
  if (cache.size > CACHE_MAX_ENTRIES) cache.delete(cache.keys().next().value as string);
}

// ask makes every request in parallel and returns the answers by id. The call
// ends within CORE_STATUS_DEADLINE_MS of `started` (default: now) however slow
// a module is — a caller asking in two phases passes the first phase's start,
// so both share one deadline.
export async function ask(request: Request, env: Env, jobs: Job[], started = Date.now()): Promise<Map<string, Answer>> {
  const timeoutMs = intVar(env.CORE_PROBE_TIMEOUT_MS, 2000);
  const deadlineMs = intVar(env.CORE_STATUS_DEADLINE_MS, 4000);
  const ttlMs = intVar(env.CORE_STATUS_CACHE_SECONDS, 15) * 1000;
  const origin = new URL(request.url).origin;
  const credential = await credentialHash(request);

  const one = async (j: Job): Promise<Answer> => {
    if ("answer" in j) return j.answer;
    const url = targetURL(origin, j.base, j.path);
    if (!url) return { status: "unreachable", ms: null, reason: "refused_target" };
    const key = `${url.href}\n${j.reader?.tag ?? ""}\n${credential}`;
    const hit = cache.get(key);
    if (hit && hit.expires > Date.now()) return { ...hit.result, cached: true };
    const left = Math.max(0, deadlineMs - (Date.now() - started));
    const result = left < timeoutMs ? await probe(url, request, left, j.reader, "deadline") : await probe(url, request, timeoutMs, j.reader);
    remember(key, result, Date.now() + ttlMs);
    return result;
  };
  const answers = await Promise.all(jobs.map(one));
  return new Map(jobs.map((j, i) => [j.id, answers[i]]));
}

// statusJob: what to ask a module to learn how it is (see the top of the file).
export function statusJob(a: Asked): Job {
  if (a.entry.id === "core") return { id: a.entry.id, answer: { status: "reachable", ms: null } };
  const q = question(a);
  if (!q) return { id: a.entry.id, answer: { status: "unknown", ms: null, reason: "no_endpoint" } };
  return { id: a.entry.id, base: a.entry.base, path: q.path, reader: q.reader };
}

// askAll asks every module how it is, within one deadline.
export function askAll(request: Request, env: Env, list: Asked[], started?: number): Promise<Map<string, Answer>> {
  return ask(request, env, list.map(statusJob), started);
}
