// The one way the home page talks to `core`: GET /api/core/<path>, and —
// for the module settings (GO-149) — send(), which carries the CSRF header
// core demands on anything but GET/HEAD (x-core-admin: 1). Unlike the shared
// navigation, the home page does NOT send x-core-quiet: with a shared
// password the browser should ask for it here, on the page whose whole job is
// administration.

export const BASE = "/api/core/";

// ApiError carries a `kind` the page turns into words and a remedy:
//   network  no answer (offline, DNS)
//   missing  404: the site has no core API (the worker is not deployed)
//   signin   401 from the `auth` mode: sign in through the auth worker
//   auth     401 otherwise: the password (the browser asks again on reload)
//   forbidden 403 (Cloudflare Access refused this account)
//   rate     429: retryAfter seconds (the password lockout)
//   config   503: core is not set up (detail says what is missing)
//   conflict 409: the settings changed meanwhile (an import preview went stale)
//   invalid  413, 422: what was sent was refused (detail says why)
//   server   5xx, and anything else unexpected
export class ApiError extends Error {
  constructor(kind, status, code, detail, retryAfter = null) {
    super(detail || code || kind);
    this.kind = kind;
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.retryAfter = retryAfter;
  }
}

// classify maps a failed response (its parsed JSON body or null) to an ApiError.
export function classify(status, body, headers) {
  const code = body && typeof body.error === "string" ? body.error : null;
  const detail = body && typeof body.detail === "string" ? body.detail : null;
  const seconds = Number.parseInt(headers.get("retry-after") ?? "", 10);
  let kind = "server";
  if (status === 404) kind = "missing";
  else if (status === 401) kind = /^Bearer/i.test(headers.get("www-authenticate") ?? "") ? "signin" : "auth";
  else if (status === 403) kind = "forbidden";
  else if (status === 429) kind = "rate";
  else if (status === 503) kind = "config";
  else if (status === 409) kind = "conflict";
  else if (status === 413 || status === 422) kind = "invalid";
  return new ApiError(kind, status, code, detail, Number.isFinite(seconds) ? seconds : null);
}

// createApi binds the wrapper to a fetch function (injectable for tests).
export const CSRF_HEADER = "x-core-admin";

export function createApi(fetchFn = (...a) => globalThis.fetch(...a)) {
  async function request(path, params, init) {
    const q = new URLSearchParams(params).toString();
    let res;
    try {
      res = await fetchFn(BASE + path + (q ? "?" + q : ""), { ...init, credentials: "same-origin" });
    } catch (e) {
      throw new ApiError("network", 0, null, String(e && e.message ? e.message : e));
    }
    const isJson = (res.headers.get("content-type") || "").includes("json");
    const data = isJson ? await res.json().catch(() => null) : null;
    if (!res.ok) throw classify(res.status, data, res.headers);
    if (data === null) throw new ApiError("server", res.status, null, "the answer was not JSON");
    return data;
  }
  const get = (path, params = {}) => request(path, params, { headers: { accept: "application/json" } });
  // send: a change. body is sent as JSON, or as it is when it is already text
  // (an import file, sent exactly as read).
  const send = (method, path, body, params = {}) =>
    request(path, params, {
      method,
      headers: { accept: "application/json", "content-type": "application/json", [CSRF_HEADER]: "1" },
      body: typeof body === "string" ? body : JSON.stringify(body),
    });
  return { get, send };
}
