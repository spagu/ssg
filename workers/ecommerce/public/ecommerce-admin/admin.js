// The panel's entry point: sign in, switch screens, load what a screen needs.

import { api, refresh, setSignedOutHandler, setToken } from "./api.js";
import { clearNotice, el, money, notify, td } from "./dom.js";
import { bindNewProductForm, bindProductFilter, loadProducts, setBaseCurrency } from "./products.js";
import { bindOrders, loadOrders } from "./orders.js";
import { bindVatForm, loadModules, loadSettings, loadVat } from "./settings.js";
import { bindOutbox, loadAudit, loadOutbox } from "./log.js";
import { bindUsers, loadGateways, loadUsers } from "./people.js";

/** Each screen: what to load, and what to call it in the top bar. */
const VIEWS = {
  dashboard: { title: "Overview", load: loadDashboard },
  products: { title: "Products", load: () => loadProducts(true) },
  orders: { title: "Orders", load: () => loadOrders(true) },
  modules: { title: "Modules", load: loadModules },
  payments: { title: "Payments", load: loadGateways },
  users: { title: "Users", load: loadUsers },
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

async function openView(name) {
  const view = VIEWS[name];
  if (!view) return;
  show(name);
  clearNotice();
  try {
    await view.load();
  } catch (err) {
    notify("error", err.message);
  }
}

function signedIn(identity) {
  document.body.dataset.state = "in";
  for (const id of ["brandbar", "topbar", "nav"]) document.getElementById(id).hidden = false;
  document.getElementById("view-login").hidden = true;

  if (identity?.testMode) {
    const badge = document.getElementById("mode-badge");
    badge.textContent = "Test mode";
    badge.title = "These are test keys. No real money moves.";
    badge.hidden = false;
  }
  if (identity?.admin?.email) {
    const who = document.getElementById("whoami");
    who.textContent = identity.admin.email;
    who.dataset.status = identity.admin.role === "owner" ? "on" : "draft";
    who.hidden = false;
  }
  if (identity?.shopName) {
    document.getElementById("brand-name").textContent = identity.shopName;
    document.title = `${identity.shopName} — shop admin`;
  }
  setBaseCurrency(identity?.baseCurrency);
  openView(window.location.hash.slice(1) || "dashboard");
}

function signedOut() {
  // The whole shell goes, not just its contents: a navigation you cannot use is
  // an invitation to click something that will answer 401.
  document.body.dataset.state = "out";
  for (const id of ["brandbar", "topbar", "nav", "mode-badge", "whoami"]) {
    document.getElementById(id).hidden = true;
  }
  for (const section of document.querySelectorAll(".view")) section.hidden = true;
  document.getElementById("view-login").hidden = false;
  document.getElementById("login-email").focus();
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

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearNotice();
    const data = new FormData(form);

    // Disabled and labelled while the request is in flight: signing in involves
    // 600 000 rounds of PBKDF2, so it is not instant, and a button that looks
    // idle gets pressed again.
    submit.disabled = true;
    submit.setAttribute("aria-busy", "true");
    const label = submit.textContent;
    submit.textContent = "Signing in…";

    try {
      const body = await api.login(String(data.get("email")), String(data.get("password")));
      form.reset();
      signedIn({ admin: { email: body.email, role: body.role } });
      // The dashboard call fills in the rest; this only needs the session.
    } catch (err) {
      notify("error", err.message);
      const password = document.getElementById("login-password");
      password.value = "";
      password.focus();
    } finally {
      submit.disabled = false;
      submit.removeAttribute("aria-busy");
      submit.textContent = label;
    }
  });

  document.getElementById("logout").addEventListener("click", async () => {
    await api.logout();
    signedOut();
    notify("ok", "Signed out.");
  });
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

async function start() {
  bindLogin();
  bindNavToggle();
  bindNewProductForm();
  bindProductFilter();
  bindUsers();
  bindOrders();
  bindVatForm();
  bindOutbox();
  setSignedOutHandler(signedOut);

  for (const button of document.querySelectorAll("#nav button")) {
    button.addEventListener("click", () => openView(button.dataset.view));
  }
  window.addEventListener("hashchange", () => {
    // A hash typed into the address bar must not open a screen for someone who
    // is not signed in. It is remembered, and used once they are.
    if (!document.getElementById("nav").hidden) openView(window.location.hash.slice(1) || "dashboard");
  });

  // Cloudflare Access mode has no login form: the request already carries a
  // verified identity, so /me answers without a token.
  try {
    const me = await api.get("/api/shop/admin/me");
    signedIn(me);
    return;
  } catch {
    // Not Access, or no session yet. A refresh cookie may still be valid.
  }

  if (await refresh()) {
    try {
      const me = await api.get("/api/shop/admin/me");
      signedIn(me);
      return;
    } catch {
      setToken(null);
    }
  }
  signedOut();
  dressLoginScreen();
}

document.addEventListener("DOMContentLoaded", start);
