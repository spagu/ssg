// The one way the panel talks to the worker. Every request goes through
// request(), so every state-changing call carries the CSRF header the worker
// demands (src/app.ts): a 403 for a missing header can only mean a bug here.
//
// URLs are relative to the panel's own address ("api/…" next to index.html),
// so the panel works under any NEWSLETTER_UI_BASE without being told it.

export const CSRF_HEADER = "x-newsletter-admin";

// ApiError carries a `kind` the views turn into words and a remedy:
//   network   the request never got an answer (offline, DNS, the worker down)
//   auth      401: credentials missing or wrong; the browser asks again on reload
//   csrf      403 for the header: a panel bug, never the admin's fault
//   forbidden 403 otherwise: Cloudflare Access refused this account
//   rate      429: retryAfter seconds (per-IP limit, test sends, the send rate)
//   install   503: the `newsletter` template's table is missing
//   config    503: a setting the action needs (site URL, transport) is missing
//   notfound  404, conflict 409, invalid 400/413/415, server 5xx
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

const KIND_BY_STATUS = { 401: "auth", 404: "notfound", 409: "conflict", 429: "rate", 400: "invalid", 413: "invalid", 415: "invalid" };

// classify maps a failed response (its parsed JSON body or null) to an ApiError.
export function classify(status, body, retryAfter) {
  const code = body && typeof body.error === "string" ? body.error : null;
  const detail = body && typeof body.detail === "string" ? body.detail : body && typeof body.hint === "string" ? body.hint : null;
  let kind = KIND_BY_STATUS[status] ?? (status >= 500 ? "server" : "invalid");
  if (status === 403) kind = code && code.includes(CSRF_HEADER) ? "csrf" : "forbidden";
  if (status === 503) kind = code === "newsletter not installed" ? "install" : "config";
  const seconds = Number.parseInt(retryAfter ?? "", 10);
  return new ApiError(kind, status, code, detail, { body, retryAfter: Number.isFinite(seconds) ? seconds : null });
}

// query turns {a: 1, b: null} into "?a=1" (empty and null values dropped).
export function query(params = {}) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== null && v !== undefined && v !== "") q.set(k, String(v));
  const s = q.toString();
  return s ? "?" + s : "";
}

// createApi binds the wrapper to a base URL ("https://example.com/newsletter-admin/")
// and a fetch function (injectable for tests).
export function createApi({ base, fetch: fetchFn = globalThis.fetch.bind(globalThis) }) {
  const url = (path, params) => new URL("api/" + path.replace(/^\//, "") + query(params), base).href;

  async function request(method, path, { params, body, raw, type } = {}) {
    const headers = { accept: "application/json" };
    if (method !== "GET" && method !== "HEAD") headers[CSRF_HEADER] = "1";
    let payload;
    if (raw !== undefined) {
      payload = raw;
      headers["content-type"] = type;
    } else if (body !== undefined) {
      payload = JSON.stringify(body);
      headers["content-type"] = "application/json";
    }
    let res;
    try {
      res = await fetchFn(url(path, params), { method, headers, body: payload, credentials: "same-origin" });
    } catch (e) {
      throw new ApiError("network", 0, null, String(e && e.message ? e.message : e));
    }
    if (res.status === 204) return null;
    const isJson = (res.headers.get("content-type") || "").includes("json");
    const data = isJson ? await res.json().catch(() => null) : null;
    if (!res.ok) throw classify(res.status, data, res.headers.get("retry-after"));
    return data;
  }

  return {
    url,
    request,
    get: (path, params) => request("GET", path, { params }),
    post: (path, body) => request("POST", path, { body: body ?? {} }),
    patch: (path, body) => request("PATCH", path, { body }),
    del: (path, body) => request("DELETE", path, { body }),
  };
}
