// The pre-send image check (#446, GET /api/campaigns/:id/images), in words.
// The worker answers with codes (problems, probe errors); this turns each
// into {key, vars} for strings/*, so the view only lays them out. The check
// itself decides nothing; with NEWSLETTER_IMAGES_REQUIRE set the worker says
// which findings block scheduling (models/image-gate.js puts that in words).

import { gateNote, isBlocking } from "./image-gate.js";

// PROBE_LIMIT mirrors MAX_PROBED_IMAGES in src/image-check.ts: how many
// different images one check tests; the rest come back with probe: null.
export const PROBE_LIMIT = 8;
// PROBE_SECONDS mirrors PROBE_TIMEOUT_MS in src/image-probe.ts.
export const PROBE_SECONDS = 5;

export const PROBLEMS = ["not_stored", "no_alt", "unreachable"];

// problemKey: the sentence for one problem code; an unknown code (a newer
// worker) still gets a sentence, never the bare code.
export const problemKey = (p) => (PROBLEMS.includes(p) ? `images.problem.${p}` : "images.problem.other");

const FETCH_ERRORS = ["timeout", "fetch_failed", "not_https", "too_many_redirects", "not_image"];

// probeNote: what the probe of one image showed — and, for a pass, what it
// does not prove. null probe = not tested (over the PROBE_LIMIT).
export function probeNote(probe) {
  if (!probe) return { key: "images.probe.skipped", vars: { max: PROBE_LIMIT } };
  if (probe.via === "store") {
    if (probe.ok) return { key: "images.probe.storeOk", vars: {} };
    if (probe.error === "not_image") return { key: "images.probe.storeNotImage", vars: {} };
    return { key: probe.status === 404 ? "images.probe.storeMissing" : "images.probe.storeFailed", vars: {} };
  }
  if (probe.ok) return { key: "images.probe.fetchOk", vars: {} };
  const vars = { agent: probe.agent ?? "", status: probe.status ?? "", type: probe.contentType || "—", seconds: PROBE_SECONDS };
  if (FETCH_ERRORS.includes(probe.error)) return { key: `images.probe.${probe.error}`, vars };
  return { key: "images.probe.status", vars };
}

// needsAttention: an image with a problem, or one the gate blocks (an image
// beyond the probe bound blocks under `load` with no problem of its own).
export const needsAttention = (i) => i.problems.length > 0 || isBlocking(i);

// checkSummary: the sentences above the list, and the callout level they
// add up to (warning when any image has a problem, critical when the
// enforced rules refuse the campaign or cannot be applied).
export function checkSummary(report) {
  const items = report.items ?? [];
  const notes = [];
  const bad = items.filter(needsAttention).length;
  if (!items.length) notes.push({ key: "images.summary.none", vars: {} });
  else if (bad) notes.push({ key: "images.summary.problems", vars: { count: bad, total: items.length } });
  else notes.push({ key: "images.summary.clean", vars: { count: items.length } });
  if (report.skipped) notes.push({ key: "images.summary.skipped", vars: { count: report.skipped, max: PROBE_LIMIT } });
  const gate = gateNote(report);
  if (gate) notes.push(gate.note);
  else if (items.length) notes.push({ key: "images.summary.warnOnly", vars: {} });
  return { notes, level: gate?.level ?? (bad ? "warning" : "ok") };
}

// checkLanguage: the language the mail is rendered in for the check — the
// campaign's first segment language, else English (the preview's default).
// Images rarely depend on it; {{language}} in an image address would.
export const checkLanguage = (c) => c?.segment?.languages?.[0] ?? "en";
