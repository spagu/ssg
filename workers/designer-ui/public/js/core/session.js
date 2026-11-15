// The panel's side of `auth` sessions. The browser is injected (`fetch`,
// `location`) so the logic runs in a test without one. Generic: `fallback`
// is what the panel assumes when api/me cannot be read, `loginTarget` its
// rule for where the login page is (both from the panel's access model).

// readMe asks api/me once. A 401 means the session went stale since the
// page loaded: reloading lets the worker send the browser to the login page.
// Any other failure gives `fallback` — the worker enforces every permission.
export async function readMe(api, location, fallback) {
  try {
    return await api.get("me");
  } catch (e) {
    if (e.kind === "auth") location.reload();
    return fallback;
  }
}

// renew is asked when an API call answered 401: the access token lives
// minutes, so ask `auth` once for a new one and say "retry"; failing that, go
// to the login page, which brings the person back to this view.
export async function renew(me, { fetch, location, loginTarget }) {
  if (me.refreshUrl) {
    try {
      const res = await fetch(me.refreshUrl, { method: "POST", headers: { "x-ssg-auth": "1" }, credentials: "same-origin" });
      if (res.ok) return true;
    } catch {
      // offline, or auth down: the login page says what is wrong
    }
  }
  const target = loginTarget(me, location.pathname + location.search + location.hash, location.origin);
  // A login URL off this site is never followed; reloading lets the worker
  // answer the request itself (its redirect to the login page, or its error).
  if (target === null) location.reload();
  else location.assign(target);
  return false;
}
