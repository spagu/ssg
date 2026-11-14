// Subscribers for the summary tests (GO-148), each carrying recognisable
// personal data a summary must never repeat.
import type { DatabaseSync } from "node:sqlite";

export const PII = ["pii-", "Piimarker", "PII-", "@", "/pii-page/"];

const ago = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString();

// seedPeople inserts n rows of a status; the first row carries every marker.
export function seedPeople(sqlite: DatabaseSync, status: string, n: number, from = 0): void {
  const stmt = sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, language, source_page, tags, consent_text, consent_at, token,
       created_at, updated_at, confirmed_at, unsubscribed_at, confirm_sent_at, ip_hash, ua_hash)
     VALUES (?, ?, ?, 'pl', '/pii-page/', 'PII-TAG', 'PII-CONSENT Alice Piimarker', ?, ?, ?, ?, ?, ?, NULL, 'PII-IPHASH', 'PII-UAHASH')`,
  );
  for (let i = from; i < from + n; i++) {
    const at = ago(i);
    stmt.run(`sub-${status}-${i}`, `pii-${status}-${i}@example.com`, status, at, `PII-TOKEN-${status}-${i}`, at, at, status === "confirmed" ? at : null, status === "unsubscribed" ? at : null);
  }
}
