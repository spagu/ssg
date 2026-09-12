// Talking to the shop's API.
//
// The access token lives in a module variable and nowhere else: not in
// localStorage, not in a cookie this script can read. A token in localStorage
// is a token any injected script can read and post elsewhere, and the refresh
// cookie (__Host-, HttpOnly, SameSite=Strict) is what survives a reload
// instead.

let accessToken = null;
let onSignedOut = () => {};

export function setSignedOutHandler(fn) {
  onSignedOut = fn;
}

export function setToken(token) {
  accessToken = token;
}

export function hasToken() {
  return Boolean(accessToken);
}

/** An error carrying the API's own code, so a caller can branch on it. */
export class ApiError extends Error {
  constructor(code, message, status) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

async function request(path, options = {}, retrying = false) {
  const headers = { accept: "application/json", ...(options.headers ?? {}) };
  if (accessToken) headers.authorization = `Bearer ${accessToken}`;
  if (options.body !== undefined && !(options.body instanceof FormData)) {
    headers["content-type"] = "application/json";
    options = { ...options, body: JSON.stringify(options.body) };
  }

  const res = await fetch(path, { ...options, headers, credentials: "same-origin" });

  // A 15-minute token expires while the panel is open. One silent refresh, and
  // only one: a refresh that itself 401s means the session is over.
  if (res.status === 401 && !retrying) {
    const refreshed = await refresh();
    if (refreshed) return request(path, options, true);
    accessToken = null;
    onSignedOut();
    throw new ApiError("unauthenticated", "Your session has ended. Sign in again.", 401);
  }

  const text = await res.text();
  const body = text ? safeJson(text) : null;
  if (!res.ok) {
    throw new ApiError(
      body?.error ?? "http_error",
      body?.message ?? `The shop answered ${res.status}.`,
      res.status,
    );
  }
  return body;
}

function safeJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

export async function refresh() {
  try {
    const res = await fetch("/api/shop/admin/auth/refresh", {
      method: "POST",
      credentials: "same-origin",
      headers: { accept: "application/json" },
    });
    if (!res.ok) return false;
    const body = await res.json();
    accessToken = body.accessToken;
    return true;
  } catch {
    return false;
  }
}

export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: "POST", body }),
  patch: (path, body) => request(path, { method: "PATCH", body }),
  put: (path, body) => request(path, { method: "PUT", body }),
  del: (path) => request(path, { method: "DELETE" }),
  upload: (path, formData) => request(path, { method: "POST", body: formData }),

  async login(email, password) {
    const body = await request("/api/shop/admin/auth/login", {
      method: "POST",
      body: { email, password },
    });
    accessToken = body.accessToken;
    return body;
  },

  async logout() {
    try {
      await request("/api/shop/admin/auth/logout", { method: "POST" });
    } finally {
      accessToken = null;
    }
  },
};
