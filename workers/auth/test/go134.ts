// Helpers for the GO-134 tests: a mail provider stand-in, invitations, and
// the pages' HTML.
import { onRequestGet as listInvites, onRequestPost as createInvite } from "../functions/api/auth/admin/invitations/index.ts";
import { onRequestPost as accept } from "../functions/api/auth/invite.ts";
import { Browser, PASSWORD, call, makeEnv } from "./fakes.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

export { listInvites, createInvite, accept };

// withMail: a worker whose mail goes to Resend (the fetch stand-in below).
export const withMail = (over: Partial<Env> = {}) => makeEnv({ AUTH_RESEND_API_KEY: "re_test_key", AUTH_MAIL_FROM: "auth@example.com", ...over });

export interface Sent {
  to: string;
  subject: string;
  text: string;
}

// captureMail replaces fetch: every Resend request is recorded and answered
// with `status` (200 unless changed). restore() puts fetch back.
export function captureMail() {
  const real = globalThis.fetch;
  const state = { status: 200, sent: [] as Sent[], urls: [] as string[] };
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    state.urls.push(String(url));
    const b = JSON.parse(String(init?.body ?? "{}")) as Sent;
    state.sent.push({ to: b.to, subject: b.subject, text: b.text });
    return new Response("{}", { status: state.status });
  }) as typeof fetch;
  return Object.assign(state, { restore: () => (globalThis.fetch = real) });
}

// linkIn finds the one-time link in a mail body.
export const linkIn = (text: string): string => (/https:\/\/\S+/.exec(text) as RegExpExecArray)[0];
export const tokenOf = (link: string): string => new URL(link).searchParams.get("token") as string;

// invite posts an invitation as `browser`; returns the answer's JSON and status.
export async function invite(env: Env, browser: Browser, json: Record<string, unknown>) {
  const res = await call(createInvite, env, "/api/auth/admin/invitations", { json, browser });
  return { status: res.status, body: await res.json() };
}

// acceptWith accepts `token` with the test password in a fresh browser.
export async function acceptWith(env: Env, token: string, extra: Record<string, unknown> = {}, browser = new Browser()) {
  const res = await call(accept, env, "/api/auth/invite", { json: { token, password: PASSWORD, ...extra }, browser });
  return { status: res.status, body: await res.json(), browser };
}

// pageOf renders a page handler and returns status, headers and HTML.
export async function pageOf(handler: unknown, env: Env, path: string, browser?: Browser, headers: Record<string, string> = {}) {
  const res = await call(handler, env, path, { browser, headers, csrf: false });
  return { res, status: res.status, html: await res.text() };
}

// table rows of a sqlite table, for scans.
export const rows = (env: Env, table: string): Record<string, unknown>[] =>
  (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { all(): Record<string, unknown>[] } } }).sqlite.prepare(`SELECT * FROM ${table}`).all();
