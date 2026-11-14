// The comments moderation panel (public/comments-admin.html). No framework,
// no build step; data is inserted as text only, never as HTML.
//
// Three admin modes, as the API decides (README, "Who can moderate"):
//   - password: the panel asks for COMMENTS_ADMIN_PASSWORD and sends it as
//     HTTP Basic on every request;
//   - Cloudflare Access: the edge has signed the moderator in already; the
//     first probe succeeds and the password form is never shown;
//   - auth (GO-136): the `auth` worker's session cookie. No password field;
//     without a session the browser goes to the login URL the API returns;
//     the panel shows who is signed in and only the actions the account may
//     perform (the API refuses the others anyway). Every request carries
//     x-ssg-auth: 1, the header the API's CSRF check wants on a write.
// Every promise is caught: a failed request says so instead of failing silently.
(function () {
  "use strict";
  var API = "/api/comments/admin";
  var REFRESH = "/api/auth/refresh"; // the auth worker's refresh (workers/auth/docs/CONTRACT.md)
  var auth = "";
  var current = "pending";
  var mode = "password";
  var perms = null; // auth mode: the permissions the account holds

  function $(id) { return document.getElementById(id); }
  function esc(node, text) { node.textContent = text; return node; }
  function may(permission) { return perms === null || perms.indexOf(permission) !== -1; }

  // headers: the CSRF header always (auth and Access modes check it on a
  // write), plus Basic with the typed password outside auth mode.
  function headers(extra) {
    var h = mode === "auth" ? { "x-ssg-auth": "1" } : { "x-ssg-auth": "1", authorization: "Basic " + btoa(":" + auth) };
    return Object.assign(h, extra || {});
  }

  // send is fetch for the panel. In auth mode a 401 gets one refresh of the
  // session and one retry; still 401 → back to the login page; a 403 names
  // the permission the account lacks.
  function send(url, init, retried) {
    init = Object.assign({ credentials: "same-origin" }, init, { headers: headers(init && init.headers) });
    return fetch(url, init).then(function (r) {
      if (mode !== "auth") return r;
      if (r.status === 401 && !retried) {
        return fetch(REFRESH, { method: "POST", credentials: "same-origin", headers: { "x-ssg-auth": "1" } })
          .then(function (f) { return f.ok ? send(url, init, true) : signIn(); });
      }
      if (r.status === 401) return signIn();
      if (r.status === 403) return r.clone().json().then(function (d) { forbidden(d.permission); return r; });
      return r;
    });
  }

  // signIn asks the API where to sign in and goes there; the URL is never built here.
  function signIn() {
    return fetch(API + "/whoami", { credentials: "same-origin" })
      .then(function (r) { return r.json(); })
      .then(function (d) { toLogin(d.login); throw new Error("signin"); });
  }

  function forbidden(permission) {
    var n = $("notice");
    n.hidden = false;
    esc(n, "Your account is missing the permission " + (permission || "for this") + ". Ask an administrator to grant it.");
  }

  function load(status) {
    current = status;
    document.querySelectorAll(".tabs button").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.getAttribute("data-status") === status));
    });
    var list = $("list");
    list.textContent = "Loading…";
    send(API + "?status=" + status, {})
      .then(function (r) {
        if (r.status === 401) throw new Error("unauthorised");
        return r.json();
      })
      .then(function (data) { render(list, data); })
      .catch(function (e) { list.textContent = e.message === "unauthorised" ? "Wrong password." : "Error loading."; });
  }

  function render(list, data) {
    list.textContent = "";
    if (!data.comments) { list.textContent = "Error loading."; return; }
    if (!data.comments.length) { list.appendChild(esc(document.createElement("p"), "Nothing here.")); return; }
    data.comments.forEach(function (c) {
      var card = document.createElement("div"); card.className = "c";
      var head = document.createElement("div"); head.className = "c-head";
      head.appendChild(esc(document.createElement("span"), c.author)).className = "c-author";
      var link = document.createElement("a"); link.href = c.url; esc(link, c.url); head.appendChild(link);
      head.appendChild(esc(document.createElement("time"), new Date(c.created_at).toLocaleString()));
      card.appendChild(head);
      card.appendChild(esc(document.createElement("div"), c.body)).className = "c-body";
      var actions = document.createElement("div"); actions.className = "c-actions";
      if (c.status !== "approved" && may("comments:moderate")) actions.appendChild(button("Approve", "approve", c.id));
      if (c.status !== "spam" && may("comments:moderate")) actions.appendChild(button("Spam", "spam", c.id));
      if (may("comments:delete")) actions.appendChild(button("Delete", "delete", c.id));
      card.appendChild(actions);
      list.appendChild(card);
    });
  }

  function button(label, action, id) {
    var b = esc(document.createElement("button"), label);
    b.addEventListener("click", function () {
      b.disabled = true;
      send(API, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ id: id, action: action }) })
        .then(function () { load(current); })
        .catch(function () { b.disabled = false; });
    });
    return b;
  }

  function open() {
    if ($("login")) $("login").hidden = true;
    $("panel").hidden = false;
    load("pending");
  }

  // toLogin follows the login URL the API names only when it stays on this
  // site: `auth` lives on the same host, so anything else is a misconfiguration
  // or a tampered answer, never a place to send the moderator.
  function toLogin(login) {
    if (!login) return;
    var url;
    try { url = new URL(login, window.location.origin); } catch (e) { return; }
    if (url.origin === window.location.origin) window.location.assign(url.pathname + url.search + url.hash);
  }

  // signedIn draws auth mode: who, and only what the account may do.
  function signedIn(who) {
    mode = "auth";
    perms = who.permissions;
    // Removed, not hidden: the form's display: grid outranks [hidden] (in the
    // password and Access modes it stays on screen after sign-in, as it always has).
    $("login").remove();
    var w = $("who");
    w.hidden = false;
    esc($("who-name"), who.user.email || who.user.id);
    $("import").hidden = !may("comments:import");
    if (may("comments:read")) open();
    else forbidden("comments:read");
  }

  $("login").addEventListener("submit", function (e) {
    e.preventDefault();
    auth = $("pw").value;
    send(API + "?status=pending", {}).then(function (r) {
      if (!r.ok) {
        $("login-msg").textContent = r.status === 401 ? "Wrong password." : "Moderation is not configured on the server.";
        return;
      }
      open();
    }).catch(function () { $("login-msg").textContent = "Network error."; });
  });
  document.querySelectorAll(".tabs button").forEach(function (b) {
    b.addEventListener("click", function () { load(b.getAttribute("data-status")); });
  });

  // Import: read a chosen file into the textarea, then POST the JSON. Accepts
  // a bare array or an { items: [...] } object; the default-status select
  // applies to any item that doesn't carry its own status.
  var fileInput = $("import-file");
  fileInput.addEventListener("change", function () {
    var f = fileInput.files && fileInput.files[0];
    if (!f) return;
    f.text().then(function (t) { $("import-json").value = t; })
      .catch(function () { $("import-msg").textContent = "Cannot read that file."; });
  });
  var runBtn = $("import-run");
  runBtn.addEventListener("click", function () {
    var msg = $("import-msg");
    var raw = $("import-json").value.trim();
    if (!raw) { msg.textContent = "Nothing to import."; return; }
    var parsed;
    try { parsed = JSON.parse(raw); } catch (e) { msg.textContent = "Invalid JSON: " + e.message; return; }
    var items = Array.isArray(parsed) ? parsed : parsed.items;
    if (!Array.isArray(items)) { msg.textContent = "Expected an array or an { items: [...] } object."; return; }
    runBtn.disabled = true;
    msg.textContent = "Importing " + items.length + "…";
    send("/api/comments/import", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ items: items, defaultStatus: $("import-status").value }),
    })
      .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d }; }); })
      .then(function (res) {
        if (!res.ok) { msg.textContent = (res.d && res.d.error) || "Import failed."; return; }
        msg.textContent = "Imported " + res.d.imported + ", " + res.d.duplicate +
          " already present, " + res.d.invalid + " skipped.";
        load(current);
      })
      .catch(function () { msg.textContent = "Network error."; })
      .finally(function () { runBtn.disabled = false; });
  });

  // First, ask who we are. Behind Cloudflare Access the edge has signed the
  // moderator in: skip the password form. In auth mode: draw for the account,
  // or refresh the session once, or go to the login page the API names. A
  // half-configured auth mode says so instead of offering a password it would
  // refuse. Anything else (the password mode): stay on the password form.
  function probe(retried) {
    return fetch(API + "/whoami", { credentials: "same-origin" }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (d) {
        if (r.ok) return d.mode === "auth" ? signedIn(d) : open();
        if (r.status === 401 && d.mode === "auth") {
          if (retried) { toLogin(d.login); return null; }
          return fetch(REFRESH, { method: "POST", credentials: "same-origin", headers: { "x-ssg-auth": "1" } })
            .then(function (f) { if (f.ok) return probe(true); toLogin(d.login); return null; });
        }
        if (d.error === "not_configured" || d.error === "auth_unavailable") {
          $("pw").closest("label").hidden = true;
          $("login").querySelector("button").hidden = true;
          $("login-msg").textContent = "Sign-in is not available: " + (d.detail || "the sign-in service cannot be reached") + ".";
        }
        return null;
      });
    });
  }
  probe(false).catch(function () { /* stay on the password form */ });
})();
