// The one way the panel talks to its worker. URLs are relative to the panel's
// own address ("api/…" next to index.html), so the panel works under any base
// path without being told it. Generic — no panel-specific import.
//
// A state-changing request is a same-origin fetch: the browser sends Origin
// and Sec-Fetch-Site, which is what the worker's CSRF rule checks. A panel
// whose worker wants a header instead passes `csrfHeader`.

// ApiError carries a `kind` the views turn into words and a remedy:
//   network    the request never got an answer (offline, the worker down)
//   auth       401: the session ended; renewing failed
//   permission 403 {error: "forbidden", permission[, keys]}: the account lacks it
//   forbidden  403 otherwise (cross_origin: a panel bug, never the person's fault)
//   rate       429: retryAfter seconds
//   config     503: the worker is missing configuration
//   notfound 404, conflict 409, invalid 400/413/415/422, server 5xx
// `code` is the worker's snake_case error; `body` its whole answer.
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

const KIND_BY_STATUS = { 401: "auth", 404: "notfound", 409: "conflict", 429: "rate", 503: "config", 400: "invalid", 413: "invalid", 415: "invalid", 422: "invalid" };

// classify maps a failed response (its parsed JSON body or null) to an ApiError.
export function classify(status, body, retryAfter) {
  const code = body && typeof body.error === "string" ? body.error : null;
  const detail = body && typeof body.detail === "string" ? body.detail : null;
  let kind = KIND_BY_STATUS[status] ?? (status >= 500 ? "server" : "invalid");
  if (status === 403) kind = code === "forbidden" && typeof body.permission === "string" ? "permission" : "forbidden";
  const seconds = Number.parseInt(retryAfter ?? "", 10);
  return new ApiError(kind, status, code, detail, {
    body,
    retryAfter: Number.isFinite(seconds) ? seconds : null,
    permission: kind === "permission" ? body.permission : null,
  });
}

// query turns {a: 1, b: null} into "?a=1" (empty and null values dropped).
export function query(params = {}) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== null && v !== undefined && v !== "") q.set(k, String(v));
  const s = q.toString();
  return s ? "?" + s : "";
}

// createApi binds the wrapper to a base URL and a fetch function (injectable
// for tests). `onUnauthorized` is asked once when a call answers 401: it
// renews the session and resolves true to retry the call, or false.
export function createApi({ base, fetch: fetchFn = globalThis.fetch.bind(globalThis), onUnauthorized = null, csrfHeader = null }) {
  const url = (path, params) => new URL("api/" + path.replace(/^\//, "") + query(params), base).href;

  async function request(method, path, { params, body } = {}, retried = false) {
    const headers = { accept: "application/json" };
    if (csrfHeader && method !== "GET") headers[csrfHeader] = "1";
    const init = { method, headers, credentials: "same-origin" };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
      headers["content-type"] = "application/json";
    }
    let res;
    try {
      res = await fetchFn(url(path, params), init);
    } catch (e) {
      throw new ApiError("network", 0, null, String(e?.message ?? e));
    }
    if (res.status === 401 && onUnauthorized && !retried && (await onUnauthorized())) return request(method, path, { params, body }, true);
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
  };
}
