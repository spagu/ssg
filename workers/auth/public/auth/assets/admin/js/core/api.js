// The one way the panel talks to the `auth` worker. Every request goes
// through request(): state-changing calls carry the CSRF header the worker
// demands (x-ssg-auth: 1, docs/SECURITY.md#csrf), a 401 renews the session
// once through POST /api/auth/refresh and retries, and every failure becomes
// an ApiError whose `kind` the views turn into words.
//
//   network     no answer at all (offline, the worker down)
//   auth        401 — the session ended; the panel sends the browser to sign in
//   stepup      403 step_up_required — a recent second factor first (stepUpUrl, minutes)
//   permission  403 forbidden {permission} — the account lacks it
//   escalation  403 escalation {permissions} — handing out more than one holds
//   forbidden   403 otherwise (csrf_check_failed is a panel bug, sensitive_needs_owner…)
//   invalid 400, notfound 404, conflict 409, rate 429, config 503, server 5xx

export const CSRF_HEADER = "x-ssg-auth";

export class ApiError extends Error {
  constructor(kind, status, code, detail, extra = {}) {
    super(detail || code || kind);
    this.kind = kind;
    this.status = status;
    this.code = code;
    this.detail = detail;
    Object.assign(this, extra);
  }
}

const KIND_BY_STATUS = { 400: "invalid", 401: "auth", 404: "notfound", 409: "conflict", 429: "rate", 503: "config" };
const KIND_BY_CODE = { step_up_required: "stepup", forbidden: "permission", escalation: "escalation" };

// classify maps a failed answer (its parsed JSON body, or null) to an ApiError.
export function classify(status, body, retryAfter) {
  const b = body && typeof body === "object" ? body : {};
  const code = typeof b.error === "string" ? b.error : null;
  const detail = typeof b.detail === "string" ? b.detail : typeof b.hint === "string" ? b.hint : null;
  let kind = KIND_BY_STATUS[status] ?? (status >= 500 ? "server" : "invalid");
  if (status === 403) kind = KIND_BY_CODE[code] ?? "forbidden";
  const seconds = Number.parseInt(retryAfter ?? "", 10);
  return new ApiError(kind, status, code, detail, {
    body: b,
    retryAfter: Number.isFinite(seconds) ? seconds : null,
    permission: typeof b.permission === "string" ? b.permission : null,
    permissions: Array.isArray(b.permissions) ? b.permissions : [],
  });
}

// query turns {a: 1, b: null} into "?a=1" (empty and null values dropped).
export function query(params = {}) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== null && v !== undefined && v !== "") q.set(k, String(v));
  const s = q.toString();
  return s ? "?" + s : "";
}

// createApi binds the wrapper to the API's base path and a fetch function
// (injectable for tests). `refresh` renews the session: POST to it once on a
// 401, then the call is retried; a second 401 stands (the session is over).
export function createApi({ base = "/api/auth/", refresh = "/api/auth/refresh", fetch: fetchFn = globalThis.fetch.bind(globalThis) } = {}) {
  const url = (path, params) => base + path.replace(/^\//, "") + query(params);

  async function renew() {
    try {
      const res = await fetchFn(refresh, { method: "POST", headers: { [CSRF_HEADER]: "1" }, credentials: "same-origin" });
      return res.ok;
    } catch {
      return false;
    }
  }

  async function request(method, path, { params, body } = {}, retried = false) {
    const headers = { accept: "application/json" };
    if (method !== "GET") headers[CSRF_HEADER] = "1";
    if (body !== undefined) headers["content-type"] = "application/json";
    let res;
    try {
      res = await fetchFn(url(path, params), { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: "same-origin" });
    } catch (e) {
      throw new ApiError("network", 0, null, String(e && e.message ? e.message : e));
    }
    if (res.status === 401 && !retried && (await renew())) return request(method, path, { params, body }, true);
    const isJson = (res.headers.get("content-type") || "").includes("json");
    const data = isJson ? await res.json().catch(() => null) : null;
    if (!res.ok) throw classify(res.status, data, res.headers.get("retry-after"));
    return data;
  }

  return {
    url,
    get: (path, params) => request("GET", path, { params }),
    post: (path, body) => request("POST", path, { body: body ?? {} }),
    put: (path, body) => request("PUT", path, { body }),
    del: (path) => request("DELETE", path),
  };
}
