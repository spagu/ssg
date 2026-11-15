// Talking to the shop's API, in whichever of the three admin modes the shop
// runs (README, "Who can use the admin panel"):
//   - password: the shared password, sent as HTTP Basic on every request. It
//     lives in this module's memory only — not in localStorage, not in a
//     cookie — so a reload asks for it again and no injected script can find
//     it lying around;
//   - Cloudflare Access: the edge signed the person in; nothing to add;
//   - auth: the `auth` worker's session cookie, which the browser sends by
//     itself. A 401 gets one refresh of the session and one retry.
// Every request carries x-ssg-auth: 1, the header the API's CSRF check wants on
// a change in the cookie modes; it costs nothing in the password mode.

let mode = "password";
let password = "";
let refreshUrl = null;
let onSignedOut = () => {};

export const CSRF = { "x-ssg-auth": "1" };
export const ME = "/api/shop/admin/me";

export function setSignedOutHandler(fn) {
  onSignedOut = fn;
}

/** The mode and the `auth` refresh URL, from /me. */
export function configure(next) {
  mode = next.mode;
  refreshUrl = next.refreshUrl ?? null;
}

export function setPassword(value) {
  password = value;
}

/** An error carrying the API's own code — and, for a 403, the permission the
 *  account lacks — so a caller can branch on it. */
export class ApiError extends Error {
  constructor(code, message, status, permission = null) {
    super(message);
    this.code = code;
    this.status = status;
    this.permission = permission;
  }
}

/** Basic credentials for the password, UTF-8 encoded, as curl -u sends them. */
export function basic(value) {
  const bytes = new TextEncoder().encode(`:${value}`);
  return `Basic ${btoa(String.fromCharCode(...bytes))}`;
}

function headersFor(extra) {
  const headers = { accept: "application/json", ...CSRF, ...extra };
  if (mode === "password" && password) headers.authorization = basic(password);
  return headers;
}

function safeJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** One refresh of the `auth` session. False when there is none to refresh.
 *  `url` is the refresh URL a 401 from /me names, before configure() ran. */
export async function refreshSession(url = refreshUrl) {
  if (!url) return false;
  try {
    const res = await fetch(url, { method: "POST", credentials: "same-origin", headers: { ...CSRF } });
    return res.ok;
  } catch {
    return false;
  }
}

/** A raw call that never throws for an HTTP status: {ok, status, body}. */
export async function call(path, options = {}) {
  let init = { ...options, headers: headersFor(options.headers), credentials: "same-origin" };
  if (options.body !== undefined && !(options.body instanceof FormData)) {
    init = { ...init, body: JSON.stringify(options.body), headers: { ...init.headers, "content-type": "application/json" } };
  }
  const res = await fetch(path, init);
  const text = await res.text();
  return { ok: res.ok, status: res.status, body: text ? safeJson(text) : null };
}

/** Turns a refused answer into the ApiError a caller branches on; a 401 also
 *  tells the panel it is signed out. */
function refusal(status, body) {
  if (status === 401) {
    onSignedOut(body);
    return new ApiError("unauthenticated", "You are signed out. Sign in again.", 401);
  }
  const b = body ?? {};
  return new ApiError(b.error ?? "http_error", b.message ?? `The shop answered ${status}.`, status, b.permission ?? null);
}

/** Whether a 401 is worth one retry: `auth` mode, first try, session refreshed. */
async function retryable(status, retrying) {
  return status === 401 && mode === "auth" && !retrying && (await refreshSession());
}

async function request(path, options = {}, retrying = false) {
  const res = await call(path, options);
  if (await retryable(res.status, retrying)) return request(path, options, true);
  if (!res.ok) throw refusal(res.status, res.body);
  return res.body;
}

/** A file (a CSV, an invoice) fetched with the panel's credentials, as a Blob.
 *  A plain link cannot do this in the password mode: the password is in this
 *  module's memory, and a browser navigation would go without it. */
async function file(path, retrying = false) {
  const res = await fetch(path, { headers: headersFor({ accept: "*/*" }), credentials: "same-origin" });
  if (await retryable(res.status, retrying)) return file(path, true);
  if (!res.ok) {
    const text = await res.text();
    throw refusal(res.status, text ? safeJson(text) : null);
  }
  return res.blob();
}

export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: "POST", body }),
  patch: (path, body) => request(path, { method: "PATCH", body }),
  put: (path, body) => request(path, { method: "PUT", body }),
  del: (path) => request(path, { method: "DELETE" }),
  upload: (path, formData) => request(path, { method: "POST", body: formData }),
  file: (path) => file(path),
  /** /me without throwing: the panel decides what to draw from the status. */
  whoami: () => call(ME),
};
