// The panel's entry point: find out who is signed in, switch screens, load what
// a screen needs.
//
// Sign-in follows the admin mode the shop runs in, as /api/shop/admin/me says:
//   - auth: no password form. Without a session the browser goes to the login
//     URL /me names — only when it is on this site (session.js, loginTarget);
//   - Cloudflare Access: the edge has signed the person in already;
//   - password: a password form; the password is sent as HTTP Basic.
// Screens and actions the account may not use are hidden (session.js); the
// API refuses them anyway and a 403 names the missing permission.

import { api, configure, refreshSession, setPassword, setSignedOutHandler } from "./api.js";
import { clearNotice, el, money, notify, td } from "./dom.js";
import { drawChannelStats } from "./channels.js";
import { bindNewProductForm, bindProductFilter, loadProducts, setBaseCurrency } from "./products.js";
import { bindOrders, loadOrders } from "./orders.js";
import { bindVatForm, loadModules, loadSettings, loadVat } from "./settings.js";
import { bindOutbox, loadAudit, loadOutbox } from "./log.js";
import { loadGateways } from "./payments.js";
import {
  VIEW_NEEDS, applyNeeds, firstView, loginTarget, may, missingPermission, setPermissions, signedInAs,
} from "./session.js";

/** Each screen: what to load, and what to call it in the top bar. */
const VIEWS = {
  dashboard: { title: "Overview", load: loadDashboard },
  products: { title: "Products", load: () => loadProducts(true) },
  orders: { title: "Orders", load: () => loadOrders(true) },
  modules: { title: "Modules", load: loadModules },
  payments: { title: "Payments", load: loadGateways },
  settings: { title: "Settings", load: loadSettings },
  vat: { title: "VAT rates", load: loadVat },
  log: { title: "Log", load: async () => {
    await loadOutbox();
    await loadAudit();
  } },
};

function show(name) {
  for (const section of document.querySelectorAll(".view")) {
    section.hidden = section.id !== `view-${name}`;
  }
  for (const button of document.querySelectorAll("#nav button")) {
    if (button.dataset.view === name) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  }
  document.getElementById("crumb").textContent = VIEWS[name]?.title ?? name;
  // The screen name is in the URL fragment, so a reload — and the browser's
  // back button — land where you were.
  if (window.location.hash !== `#${name}`) window.location.hash = name;
  // A screen change means a new page as far as a reader is concerned.
  document.getElementById("main").scrollTo?.({ top: 0 });
}

async function openView(wanted) {
  const name = firstView(wanted);
  const view = VIEWS[name];
  if (!view) {
    notify("error", missingPermission("ecommerce:read"));
    return;
  }
  show(name);
  clearNotice();
  try {
    await view.load();
  } catch (err) {
    notify("error", err.permission ? missingPermission(err.permission) : err.message);
  }
}

/** /me as it answered at sign-in. */
let current = null;

function signedIn(me) {
  current = me;
  configure(me);
  setPermissions(me.permissions);
  document.body.dataset.state = "in";
  for (const id of ["brandbar", "topbar", "nav"]) document.getElementById(id).hidden = false;
  document.getElementById("view-login").hidden = true;

  // Nothing to sign out of with Access: the edge holds that session.
  document.getElementById("logout").hidden = me.mode === "access";
  for (const button of document.querySelectorAll("#nav button")) {
    button.closest("li").hidden = !may(VIEW_NEEDS[button.dataset.view]);
  }
  applyNeeds(document.body);

  if (me.testMode) {
    const badge = document.getElementById("mode-badge");
    badge.textContent = "Test mode";
    badge.title = "These are test keys. No real money moves.";
    badge.hidden = false;
  }
  const who = document.getElementById("whoami");
  who.textContent = signedInAs(me);
  who.title = me.mode === "auth" ? `Signed in with the auth worker as ${me.user.email}` : "";
  who.hidden = false;
  if (me.shopName) {
    document.getElementById("brand-name").textContent = me.shopName;
    document.title = `${me.shopName} — shop admin`;
  }
  setBaseCurrency(me.baseCurrency);
  void openView(window.location.hash.slice(1) || "dashboard");
}

/** Back to the password form (the password mode), or to the login page the API
 *  names (auth mode). */
function signedOut(body) {
  if (body?.mode === "auth") {
    goToLogin(body.loginUrl);
    return;
  }
  // The whole shell goes, not just its contents: a navigation you cannot use is
  // an invitation to click something that will answer 401.
  setPassword("");
  document.body.dataset.state = "out";
  for (const id of ["brandbar", "topbar", "nav", "mode-badge", "whoami"]) {
    document.getElementById(id).hidden = true;
  }
  for (const section of document.querySelectorAll(".view")) section.hidden = true;
  document.getElementById("view-login").hidden = false;
  document.getElementById("login-password").focus();
}

function goToLogin(loginUrl) {
  const target = loginTarget(loginUrl, window.location.origin);
  if (target) window.location.assign(target);
  else notify("error", "Sign-in is configured to leave this site, so the panel will not follow it. Check SHOP_AUTH_LOGIN_URL.");
}

/** What the sign-in screen can say before anyone has signed in.
 *
 *  The catalogue endpoint is public, so the card can carry the shop's own name
 *  rather than a bare "Shop admin". Decoration: a shop that cannot answer
 *  leaves the defaults in place. */
async function dressLoginScreen() {
  try {
    const res = await fetch("/api/shop/products", { headers: { accept: "application/json" } });
    if (!res.ok) return;
    const { shop } = await res.json();
    if (shop?.name) {
      document.getElementById("auth-shop").textContent = shop.name;
      document.title = `${shop.name} — shop admin`;
    }
  } catch {
    // Offline, or the API is not deployed yet. The card stands as written.
  }
}

async function loadDashboard() {
  const [me, stats] = await Promise.all([
    api.get("/api/shop/admin/me"),
    api.get("/api/shop/admin/stats?window=30d"),
  ]);

  const warnings = document.getElementById("warnings");
  warnings.replaceChildren(...(me.warnings ?? []).map((w) => el("li", { text: w })));

  // The count beside Orders is the one number worth carrying into every screen:
  // an order stuck in review is a buyer waiting.
  const badge = document.getElementById("review-count");
  badge.textContent = String(stats.needsReview ?? 0);
  badge.hidden = !stats.needsReview;

  const figures = [];
  for (const row of stats.byCurrency) {
    figures.push([`Sales, 30 days (${row.currency})`, money(row.netMinor, row.currency)]);
    figures.push([`VAT collected (${row.currency})`, money(row.taxMinor, row.currency)]);
    figures.push([`Orders (${row.currency})`, String(row.orders)]);
  }
  if (stats.byCurrency.length === 0) figures.push(["Sales, 30 days", "nothing yet"]);
  figures.push(["Needs review", String(stats.needsReview)]);
  figures.push(["Stuck messages", String(stats.stuckMessages)]);

  document.getElementById("stats").replaceChildren(
    ...figures.map(([label, value]) =>
      el("dl", { class: "stat" }, el("dt", { text: label }), el("dd", { text: value })),
    ),
  );

  drawChannelStats(document.getElementById("by-channel"), stats.byChannel ?? []);

  const top = document.getElementById("top-products");
  top.replaceChildren(
    ...stats.topProducts.map((p) =>
      el("tr", {},
        td("Product", p.name),
        td("Units", { class: "numeric" }, String(p.units)),
        td("Revenue", { class: "numeric" }, money(p.revenue, stats.baseCurrency)),
      ),
    ),
  );
  if (stats.topProducts.length === 0) {
    top.replaceChildren(el("tr", {}, el("td", { colspan: "3", class: "muted", text: "Nothing sold yet." })));
  }

  const countries = document.getElementById("by-country");
  countries.replaceChildren(
    ...stats.byCountry.map((c) =>
      el("tr", {},
        td("Country", c.country),
        td("Orders", { class: "numeric" }, String(c.orders)),
        td("Gross", { class: "numeric" }, money(c.gross, stats.baseCurrency)),
      ),
    ),
  );
  if (stats.byCountry.length === 0) {
    countries.replaceChildren(el("tr", {}, el("td", { colspan: "3", class: "muted", text: "No paid orders yet." })));
  }
}

function bindLogin() {
  const form = document.getElementById("login-form");
  const submit = document.getElementById("login-submit");

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void signInWithPassword(form, submit);
  });

  document.getElementById("logout").addEventListener("click", signOut);
}

async function signInWithPassword(form, submit) {
  clearNotice();
  const input = document.getElementById("login-password");
  submit.disabled = true;
  submit.setAttribute("aria-busy", "true");
  try {
    setPassword(input.value);
    const res = await api.whoami();
    if (res.ok) {
      form.reset();
      signedIn(res.body);
      return;
    }
    setPassword("");
    notify("error", res.body?.message ?? `The shop answered ${res.status}.`);
    input.value = "";
    input.focus();
  } catch (err) {
    notify("error", err.message);
  } finally {
    submit.disabled = false;
    submit.removeAttribute("aria-busy");
  }
}

/** Signing out: in auth mode a form POST to the auth worker's logout, which
 *  clears the session cookies and brings the browser back here; in the
 *  password mode the password is forgotten. */
function signOut() {
  const logoutUrl = current?.mode === "auth" ? loginTarget(current.logoutUrl, window.location.origin) : null;
  if (logoutUrl) {
    const form = el("form", { method: "post", action: logoutUrl, hidden: true },
      el("input", { type: "hidden", name: "next", value: "/ecommerce-admin/" }));
    document.body.append(form);
    form.submit();
    return;
  }
  current = null;
  signedOut(null);
  notify("ok", "Signed out.");
}

/** The sidebar collapses to a rail of icons, and stays that way.
 *
 *  Remembered per browser: someone who works on a laptop and wants the width
 *  should not have to say so on every visit. localStorage can throw — a private
 *  window, blocked site data — so every touch of it is guarded and the panel
 *  simply opens expanded when it cannot remember. */
const NAV_KEY = "shop-admin-nav";

function applyNavState(collapsed) {
  const toggle = document.getElementById("nav-toggle");
  document.body.dataset.nav = collapsed ? "collapsed" : "expanded";
  toggle.setAttribute("aria-expanded", String(!collapsed));
  const label = collapsed ? "Expand the menu" : "Collapse the menu";
  toggle.setAttribute("aria-label", label);
  toggle.title = label;

  // With the words hidden, each button's accessible name has to come from
  // somewhere: it comes from the same words.
  for (const button of document.querySelectorAll("#nav button")) {
    const text = button.querySelector(".navtext")?.textContent ?? "";
    if (collapsed) {
      button.setAttribute("aria-label", text);
      button.title = text;
    } else {
      button.removeAttribute("aria-label");
      button.removeAttribute("title");
    }
  }
}

function bindNavToggle() {
  let collapsed = false;
  try {
    collapsed = localStorage.getItem(NAV_KEY) === "collapsed";
  } catch (e) {
    console.info("shop admin: menu state not remembered", e.name);
  }
  applyNavState(collapsed);

  document.getElementById("nav-toggle").addEventListener("click", () => {
    collapsed = !collapsed;
    applyNavState(collapsed);
    try {
      localStorage.setItem(NAV_KEY, collapsed ? "collapsed" : "expanded");
    } catch {
      // The choice holds for this visit, which is better than refusing it.
    }
  });
}

/** The first question: who is this? /me answers in every mode, and its status
 *  says which sign-in to offer. */
async function probe(retried) {
  const res = await api.whoami();
  if (res.ok) {
    signedIn(res.body);
    return;
  }
  const body = res.body ?? {};
  if (res.status === 401 && body.mode === "auth") {
    // The access token lives minutes; the session behind it, days.
    if (!retried && (await refreshSession(body.refreshUrl))) return probe(true);
    goToLogin(body.loginUrl);
    return;
  }
  if (res.status === 401 && body.error === "unauthorized") {
    signedOut(null);
    await dressLoginScreen();
    return;
  }
  // Not configured, half configured, Access refusing: say so instead of
  // offering a password the shop would refuse.
  signedOut(null);
  document.getElementById("login-form").hidden = true;
  notify("error", body.message ?? body.detail ?? `Sign-in is not available (the shop answered ${res.status}).`);
}

async function start() {
  bindLogin();
  bindNavToggle();
  bindNewProductForm();
  bindProductFilter();
  bindOrders();
  bindVatForm();
  bindOutbox();
  setSignedOutHandler(signedOut);

  // Actions a script builds later are checked as they appear.
  new MutationObserver((changes) => {
    for (const change of changes) {
      for (const node of change.addedNodes) if (node.nodeType === 1) applyNeeds(node);
    }
  }).observe(document.body, { childList: true, subtree: true });

  for (const button of document.querySelectorAll("#nav button")) {
    button.addEventListener("click", () => {
      void openView(button.dataset.view);
    });
  }
  window.addEventListener("hashchange", () => {
    // A hash typed into the address bar must not open a screen for someone who
    // is not signed in. It is remembered, and used once they are.
    if (!document.getElementById("nav").hidden) void openView(window.location.hash.slice(1) || "dashboard");
  });

  try {
    await probe(false);
  } catch (err) {
    notify("error", err.message);
  }
}

document.addEventListener("DOMContentLoaded", () => {
  void start();
});
