// The content rules (#449, GET /api/campaigns/:id/content-check and the
// 409 content_check_failed of scheduling), in words. The worker answers with
// findings — {rule, where, value, …} — and this turns each into {key, vars}
// for strings/*, so the view only lays them out. Phrases and URLs stay data:
// they go into the sentence as variables, never as markup.

export const RULES = ["banned_phrase", "blocked_image", "utm_conflict"];
const PHRASE_PLACES = new Set(["subject", "body", "text"]);
const LINK_PLACES = new Set(["body", "text"]);

// rulesShown: whether the editor shows the "Content rules" card — only when
// a rule is configured, or the configuration has a problem to show.
export const rulesShown = (rules) => Boolean(rules?.active);

// findingNote: one finding as a sentence key and its variables. A finding the
// panel does not know (a newer worker) still gets a sentence.
export function findingNote(f) {
  if (f.rule === "banned_phrase" && PHRASE_PLACES.has(f.where)) {
    return { key: `content.finding.phrase.${f.where}`, vars: { phrase: f.value, match: f.match ?? f.value, count: f.count ?? 1 } };
  }
  if (f.rule === "blocked_image") return { key: "content.finding.image", vars: { url: f.value, pattern: f.pattern ?? "" } };
  if (f.rule === "utm_conflict" && LINK_PLACES.has(f.where)) {
    return { key: `content.finding.utm.${f.where}`, vars: { url: f.value, host: f.host ?? "", parameters: (f.parameters ?? []).join(", ") } };
  }
  return { key: "content.finding.other", vars: {} };
}

// rulesNotes: what is configured, as sentences (counts only: the panel never
// sees the phrase list).
export function rulesNotes(rules) {
  const notes = [];
  if (rules.bannedPhrases) notes.push({ key: "content.rules.phrases", vars: { count: rules.bannedPhrases } });
  if (rules.blockedImagePatterns) notes.push({ key: "content.rules.patterns", vars: { count: rules.blockedImagePatterns } });
  if (rules.utmStrict) notes.push({ key: "content.rules.utm", vars: {} });
  return notes;
}

// checkSummary: the sentence above the findings and the callout level.
export function checkSummary(findings) {
  return findings.length
    ? { note: { key: "content.summary.problems", vars: { count: findings.length } }, level: "critical" }
    : { note: { key: "content.summary.clean", vars: {} }, level: "ok" };
}

// refusedFindings: the findings of a refused schedule (an ApiError of the
// panel's api.js), or null when the error is another one.
export function refusedFindings(err) {
  const findings = err?.code === "content_check_failed" ? err.body?.findings : null;
  return Array.isArray(findings) ? findings : null;
}
