// The panel's side of `auth` sessions (GO-137). The browser is injected
// (`fetch`, `location`) so the logic runs in a test without one.

import { ALL, loginTarget } from "../models/access.js";

// readMe asks api/me once. A 401 naming a session `reason` means `auth` mode
// and a session gone stale since the page loaded: reloading lets the worker
// send the browser to the login page. Any other failure leaves the panel as
// it always was (ALL) — the worker enforces every permission anyway.
export async function readMe(api, location) {
  try {
    return await api.get("me");
  } catch (e) {
    if (e.kind === "auth" && e.body && e.body.reason) location.reload();
    return ALL;
  }
}

// renew is asked when an API call answered 401. Outside `auth` mode it says no
// (the 401 stands, as it always did). In `auth` mode the access token lives
// minutes: ask `auth` once for a new one and say "retry"; failing that, go to
// the login page, which brings the admin back to this view.
export async function renew(me, { fetch, location }) {
  if (me.mode !== "auth") return false;
  try {
    const res = await fetch(me.refreshUrl, { method: "POST", headers: { "x-ssg-auth": "1" }, credentials: "same-origin" });
    if (res.ok) return true;
  } catch {
    // offline, or auth down: the login page says what is wrong
  }
  location.assign(loginTarget(me, location.pathname + location.search + location.hash));
  return false;
}
