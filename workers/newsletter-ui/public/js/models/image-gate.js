// The image gate (#448) in words. NEWSLETTER_IMAGES_REQUIRE on the worker
// names the rules (alt, store, load) whose findings refuse scheduling; the
// worker reports them in GET /api/settings (imageRules: {rules, problem}), in
// GET /api/campaigns/:id/images (rules, rulesProblem, item.blocking) and in a
// refused schedule (409 image_check_failed: {items, rules}). This turns those
// into {key, vars} for strings/*; the views only lay them out. No DOM here.

export const RULES = ["alt", "store", "load"];

// ruleKey: the name of one rule; an unknown one (a newer worker) still gets words.
export const ruleKey = (r) => (RULES.includes(r) ? `imageGate.rule.${r}` : "imageGate.rule.other");

// ruleNames: the enforced rules as one phrase for {rules} in a sentence.
export const ruleNames = (t, rules = []) => rules.map((r) => t(ruleKey(r))).join(", ");

// gateMode: "off" (warnings only), "on", "broken" (the rules cannot apply:
// every schedule is refused) or "unknown" (the settings could not be read).
// `info` is settings.imageRules ({rules, problem}) or a check report.
export function gateMode(info) {
  if (!info) return "unknown";
  if (info.problem || info.rulesProblem) return "broken";
  return info.rules?.length ? "on" : "off";
}

// leadNote: the card's lead, true in every mode. Off, it is the 1.9.15 text
// (the check only warns).
export function leadNote(info) {
  const mode = gateMode(info);
  if (mode === "off") return { key: "images.lead", vars: {} };
  if (mode === "broken") return { key: "imageGate.lead.broken", vars: { detail: info.problem || info.rulesProblem } };
  if (mode === "on") return { key: "imageGate.lead.on", vars: {}, rules: info.rules };
  return { key: "imageGate.lead.unknown", vars: {} };
}

// isBlocking: an image the enforced rules refuse.
export const isBlocking = (item) => (item.blocking?.length ?? 0) > 0;

// gateNote: the sentence that replaces "only a warning" when the answer
// carries rules, with the callout level it calls for; null when it does not
// (the switch is off: the caller keeps its warning-only sentence).
export function gateNote(report) {
  if (report.rulesProblem) return { note: { key: "imageGate.summary.broken", vars: { detail: report.rulesProblem } }, level: "critical" };
  if (!report.rules?.length) return null;
  const blocking = (report.items ?? []).filter(isBlocking).length;
  if (blocking) return { note: { key: "imageGate.summary.blocking", vars: { count: blocking }, rules: report.rules }, level: "critical" };
  return { note: { key: "imageGate.summary.passes", vars: {}, rules: report.rules }, level: null };
}

// itemProblems: each problem of one image, marked when it blocks. An image
// beyond the probe bound has not_probed only among its blocking problems.
export function itemProblems(item) {
  const blocking = item.blocking ?? [];
  const all = [...item.problems, ...blocking.filter((p) => !item.problems.includes(p))];
  return all.map((code) => ({ code, blocking: blocking.includes(code) }));
}

// refusal reads a failed schedule: the 409 image_check_failed body as a
// report the image views can draw (every item blocking), or null for any
// other failure.
export function refusal(err) {
  const body = err?.body;
  if (err?.code !== "image_check_failed" || !Array.isArray(body?.items)) return null;
  const rules = Array.isArray(body.rules) ? body.rules : [];
  const items = body.items.map((i) => ({ ...i, problems: i.problems ?? [], blocking: i.blocking ?? i.problems ?? [] }));
  return { items, rules, note: { key: "imageGate.refused", vars: { count: items.length }, rules } };
}
