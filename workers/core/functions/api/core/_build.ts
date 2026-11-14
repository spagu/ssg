// The site's own build report: build.json, which ssg writes into the output
// when `build_report` is set (docs/CONFIGURATION.md) and the site publishes
// as a static file. A leading underscore keeps this file out of the Pages
// route table.
//
// It is fetched like everything else core asks for (same host, no redirect,
// timeout, size cap, only the caller's credential — a site behind Access
// needs it) and read field by field: only the known fields, each of its own
// type, are passed on. Absent is normal — the option is off by default.

import type { Env } from "./_lib";
import type { Reader } from "./_probe";
import type { Answer, Job } from "./_status";
import { joinPath } from "./_target";

export const DEFAULT_BUILD_REPORT = "/build.json";
export const BUILD_OPTION = "build_report";
const MAX_BYTES = 64 * 1024;
const ISO = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
const WORD = /^[A-Za-z0-9._+-]{1,64}$/;
const KEY = /^[a-z0-9_-]{1,40}$/;

export interface BuildFacts {
  generator: string | null;
  version: string | null;
  buildTime: string | null;
  finished: string | null;
  durationSeconds: number | null;
  pages: number | null;
  files: number | null;
  content: Record<string, number>;
}
export type BuildState = "ok" | "absent" | "unreadable" | "invalid";

const word = (v: unknown): string | null => (typeof v === "string" && WORD.test(v) ? v : null);
const time = (v: unknown): string | null => (typeof v === "string" && ISO.test(v) && !Number.isNaN(Date.parse(v)) ? v : null);
const count = (v: unknown): number | null => (Number.isSafeInteger(v) && (v as number) >= 0 ? (v as number) : null);
const seconds = (v: unknown): number | null => (typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : null);

// readBuild keeps the known fields of a report and drops everything else.
export function readBuild(raw: unknown): BuildFacts | null {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return null;
  const r = raw as Record<string, unknown>;
  const content: Record<string, number> = {};
  if (typeof r.content === "object" && r.content !== null && !Array.isArray(r.content)) {
    for (const [k, v] of Object.entries(r.content).slice(0, 50)) {
      const n = count(v);
      if (KEY.test(k) && n !== null) content[k] = n;
    }
  }
  const facts: BuildFacts = {
    generator: word(r.generator),
    version: word(r.version),
    buildTime: time(r.buildTime),
    finished: time(r.finished),
    durationSeconds: seconds(r.durationSeconds),
    pages: count(r.pages),
    files: count(r.files),
    content,
  };
  return facts.version === null && facts.buildTime === null ? null : facts;
}

export const buildReader: Reader = {
  tag: "build",
  max: MAX_BYTES,
  invalid: "invalid",
  read(raw) {
    const facts = readBuild(raw);
    return facts ? { ok: true, found: { value: facts } } : { ok: false, detail: "not an ssg build report (no version, no buildTime)" };
  },
};

// buildJob: the request for the report. CORE_BUILD_REPORT_PATH names another
// file when build_report does; anything but a plain path falls back.
export function buildJob(env: Env): Job {
  const wanted = env.CORE_BUILD_REPORT_PATH?.trim() || DEFAULT_BUILD_REPORT;
  const path = joinPath("/", wanted) === null || /[?#]/.test(wanted) ? DEFAULT_BUILD_REPORT : wanted;
  return { id: "build", base: "/", path, reader: buildReader };
}

// buildView: the facts, or why there are none. A 404 is "absent" — the
// option is off — and the page says how to turn it on.
export function buildView(answer: Answer): { build: BuildFacts | null; buildState: BuildState } {
  if (answer.status === "reachable") return { build: answer.value as BuildFacts, buildState: "ok" };
  if (answer.status === "invalid") return { build: null, buildState: "invalid" };
  return { build: null, buildState: answer.httpStatus === 404 ? "absent" : "unreadable" };
}
