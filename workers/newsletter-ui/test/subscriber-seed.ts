// Fixtures shared by the GO-124 tests: rows as the `newsletter` template
// writes them, and a call through the whole pipeline (auth, CSRF, routing).
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

export const B = "/newsletter-admin";

export interface SeedRow {
  id?: string;
  email: string;
  status?: string;
  language?: string | null;
  tags?: string;
  created_at?: string;
  unsubscribed_at?: string | null;
  confirmed_at?: string | null;
}

let n = 0;
// seed inserts subscribers with the template's defaults; returns their ids.
export function seed(d1: FakeD1, rows: SeedRow[]): string[] {
  const stmt = d1.sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, language, source_page, tags, consent_text, consent_at, token,
       created_at, updated_at, confirmed_at, unsubscribed_at, confirm_sent_at, ip_hash, ua_hash)
     VALUES (?, ?, ?, ?, '/blog/', ?, 'I agree to the newsletter', ?, ?, ?, ?, ?, ?, NULL, 'iphash-secret', 'uahash-secret')`,
  );
  return rows.map((r) => {
    n++;
    const id = r.id ?? `id-${String(n).padStart(6, "0")}`;
    const at = r.created_at ?? "2026-01-01T00:00:00.000Z";
    const token = ("tok" + n).padEnd(43, "x");
    stmt.run(id, r.email, r.status ?? "confirmed", r.language === undefined ? "en" : r.language, r.tags ?? "", at, token, at, at, r.confirmed_at ?? null, r.unsubscribed_at ?? null);
    return id;
  });
}

// readyEnv is makeEnv with this worker's tables already created.
export async function readyEnv(overrides: Record<string, unknown> = {}) {
  const env = makeEnv(overrides);
  await ensureSchema(env as never);
  return env;
}

// api sends a request through handleFetch with valid credentials.
export async function api(env: Record<string, unknown>, method: string, path: string, body?: unknown, headers: Record<string, string> = {}) {
  const init: RequestInit & { auth: string } = { method, auth: PASSWORD, headers };
  if (body !== undefined) {
    init.body = typeof body === "string" ? body : JSON.stringify(body);
    if (!headers["content-type"]) init.headers = { "content-type": typeof body === "string" ? "text/csv" : "application/json", ...headers };
  }
  return handleFetch(req(B + path, init), env as never, fakeCtx() as never);
}

export const row = (d1: FakeD1, id: string) => d1.sqlite.prepare("SELECT * FROM subscribers WHERE id = ?").get(id) as Record<string, unknown> | undefined;
