// The subscriber, import and suppression screens' rules, without DOM:
// filters ↔ query strings, the import's column mapping, what a destructive
// action says before it runs.

export const SUB_STATUSES = ["confirmed", "pending", "unsubscribed"];
export const REASONS = ["hard", "soft", "complaint", "manual"];

// The roles an import column can play (src/import-columns.ts ROLES).
export const ROLES = ["email", "status", "signedUpAt", "unsubscribedAt", "bouncedAt", "language", "tags", "sourcePage"];

// Import outcome classes, in the order the preview lists them.
export const CLASSES = ["toConfirm", "toPending", "toUnsubscribed", "toSuppress"];
export const SKIPS = ["duplicate", "existing", "suppressed", "invalid"];

// cleanFilters keeps only filters the API accepts.
export function cleanFilters(f = {}) {
  const out = {};
  if (SUB_STATUSES.includes(f.status)) out.status = f.status;
  if (f.language && /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/.test(f.language)) out.language = f.language;
  if (f.tag && /^[a-z0-9_-]{1,32}$/.test(f.tag.trim().toLowerCase())) out.tag = f.tag.trim().toLowerCase();
  if (f.q && f.q.trim()) out.q = f.q.trim().slice(0, 254);
  return out;
}

// tagsFrom("news, Offers") → {tags: ["news", "offers"]} or {error: "<bad tag>"}.
export function tagsFrom(text) {
  const tags = [
    ...new Set(
      String(text || "")
        .split(/[,;]/)
        .map((s) => s.trim().toLowerCase())
        .filter(Boolean),
    ),
  ];
  const bad = tags.find((t) => !/^[a-z0-9_-]{1,32}$/.test(t));
  return bad ? { error: bad } : { tags };
}

// headersOf(detected) → every header the file has, in a stable order: the
// ones used for a role first (role order), then the ignored ones.
export function headersOf(detected) {
  const seen = new Set();
  for (const r of ROLES) if (detected.columns[r]) seen.add(detected.columns[r]);
  for (const i of detected.ignored) seen.add(i.header);
  return [...seen];
}

// mappingFrom(detected, chosen) → the `mapping` to send: only roles whose
// column differs from what was detected; "" = "no column for this role".
export function mappingFrom(detected, chosen) {
  const out = {};
  for (const r of ROLES) {
    if (!(r in chosen)) continue;
    const want = chosen[r] || null;
    if (want !== (detected.columns[r] ?? null)) out[r] = want;
  }
  return out;
}

// duplicateColumn: a header chosen for two roles (the worker refuses it).
export function duplicateColumn(chosen) {
  const seen = new Set();
  for (const r of ROLES) {
    const v = chosen[r];
    if (!v) continue;
    if (seen.has(v)) return v;
    seen.add(v);
  }
  return null;
}

// delimiterKey(";") → "delimiter.semicolon".
export const delimiterKey = (d) => ({ ",": "delimiter.comma", ";": "delimiter.semicolon", "\t": "delimiter.tab" })[d] ?? "delimiter.other";

// ignoredKey: the worker's English reason for an ignored column → a string key
// ("names" — never stored; "lost" — another column won the role; "other").
export const ignoredKey = (reason) =>
  `import.reason.${/does not store names/.test(reason) ? "names" : /was used for/.test(reason) ? "lost" : "other"}`;

// importBody builds the JSON body for preview and run.
export function importBody({ csv, consentSource = "", confirmed = false, dateOrder = null, mapping = {} }) {
  const body = { csv, consentSource: consentSource.trim(), confirmed: !!confirmed, mapping };
  if (dateOrder === "dmy" || dateOrder === "mdy") body.dateOrder = dateOrder;
  return body;
}

// needsConsent mirrors the worker: a consent source is required unless every
// row to write is an unsubscribe or a suppression.
export const needsConsent = (preview) => {
  const c = preview.classCounts;
  return c.toConfirm + c.toPending > 0 || c.toConfirm + c.toPending + c.toUnsubscribed + c.toSuppress === 0;
};

// suppressionName: the address, or words for an erased (hash-only) row.
export const suppressionName = (s, t) => s.email ?? t("sup.erased", { hash: s.emailHash.slice(0, 10) });

// liftNeedsConfirm: lifting a complaint must be said out loud (the worker
// answers 409 confirm_required otherwise).
export const liftNeedsConfirm = (s) => s.reason === "complaint";

// sendStatusKey: words for a campaign_sends row's status.
export const sendStatusKey = (status) => `send.${status}`;

// readFileText reads an uploaded file as text. UTF-16 files (with a BOM) are
// decoded here, so the JSON body always carries proper text.
export async function readFileText(file) {
  const buf = new Uint8Array(await file.arrayBuffer());
  if (buf[0] === 0xff && buf[1] === 0xfe) return new TextDecoder("utf-16le").decode(buf.subarray(2));
  if (buf[0] === 0xfe && buf[1] === 0xff) return new TextDecoder("utf-16be").decode(buf.subarray(2));
  return new TextDecoder("utf-8").decode(buf).replace(/^﻿/, "");
}
