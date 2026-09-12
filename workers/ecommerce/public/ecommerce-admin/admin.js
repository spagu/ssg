// The panel's entry point: sign in, switch screens, load what a screen needs.

import { api, refresh, setSignedOutHandler, setToken } from "./api.js";
import { clearNotice, el, money, notify, td } from "./dom.js";
import { bindNewProductForm, loadProducts, setBaseCurrency } from "./products.js";
import { bindOrders, loadOrders } from "./orders.js";
import { bindVatForm, loadSettings, loadVat } from "./settings.js";
import { bindOutbox, loadAudit, loadOutbox } from "./log.js";

const VIEWS = {
  dashboard: loadDashboard,
  products: loadProducts,
  orders: () => loadOrders(true),
  settings: loadSettings,
  vat: loadVat,
  log: async () => {
    await loadOutbox();
    await loadAudit();
  },
};

function show(name) {
  for (const section of document.querySelectorAll(".view")) {
    section.hidden = section.id !== `view-${name}`;
  }
  for (const button of document.querySelectorAll("#tabs button")) {
    if (button.dataset.view === name) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  }
  // The screen name is in the URL fragment, so a reload — and the browser's
  // back button — land where you were.
  if (window.location.hash !== `#${name}`) window.location.hash = name;
}

async function openView(name) {
  const load = VIEWS[name];
  if (!load) return;
  show(name);
  try {
    await load();
  } catch (err) {
    notify("error", err.message);
  }
}

function signedIn(identity) {
  document.body.dataset.state = "in";
  document.getElementById("topbar").hidden = false;
  document.getElementById("nav").hidden = false;
  document.getElementById("logout").hidden = false;
  document.getElementById("view-login").hidden = true;
  if (identity?.testMode) {
    const badge = document.getElementById("mode-badge");
    badge.textContent = "Test mode — no real money";
    badge.hidden = false;
  }
  if (identity?.shopName) document.title = `${identity.shopName} — shop admin`;
  setBaseCurrency(identity?.baseCurrency);
  openView(window.location.hash.slice(1) || "dashboard");
}

function signedOut() {
  // The whole shell goes, not just its contents: a navigation you cannot use is
  // an invitation to click something that will answer 401.
  document.body.dataset.state = "out";
  document.getElementById("topbar").hidden = true;
  document.getElementById("nav").hidden = true;
  document.getElementById("logout").hidden = true;
  document.getElementById("mode-badge").hidden = true;
  for (const section of document.querySelectorAll(".view")) section.hidden = true;
  document.getElementById("view-login").hidden = false;
  document.getElementById("login-email").focus();
}

/** What the sign-in screen can say before anyone has signed in.
 *
 *  The catalogue endpoint is public, so the card can carry the shop's own name
 *  and warn that these are test keys — both of which a seller looking at a bare
 *  "Shop admin" would have to guess at. It is decoration: a shop that cannot
 *  answer leaves the defaults in place. */
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

  const cards = document.getElementById("stats");
  const figures = [];
  for (const row of stats.byCurrency) {
    figures.push([`Sales (30 days, ${row.currency})`, money(row.netMinor, row.currency)]);
    figures.push([`VAT collected (${row.currency})`, money(row.taxMinor, row.currency)]);
    figures.push([`Orders (${row.currency})`, String(row.orders)]);
  }
  if (stats.byCurrency.length === 0) figures.push(["Sales (30 days)", "nothing yet"]);
  figures.push(["Needs review", String(stats.needsReview)]);
  figures.push(["Stuck messages", String(stats.stuckMessages)]);
  figures.push(["Signed in as", me.admin?.email ?? "—"]);

  cards.replaceChildren(
    ...figures.map(([label, value]) =>
      el("dl", { class: "card" }, el("dt", { text: label }), el("dd", { text: value })),
    ),
  );

  document.getElementById("top-products").replaceChildren(
    ...stats.topProducts.map((p) =>
      el("tr", {}, td("Product", p.name), td("Units", String(p.units)), td("Revenue", money(p.revenue, stats.baseCurrency))),
    ),
  );
  document.getElementById("by-country").replaceChildren(
    ...stats.byCountry.map((c) =>
      el("tr", {}, td("Country", c.country), td("Orders", String(c.orders)), td("Gross", money(c.gross, stats.baseCurrency))),
    ),
  );
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
      signedIn({ testMode: false, baseCurrency: null, admin: { email: body.email } });
      // The dashboard call fills in the rest; this only needs the session.
    } catch (err) {
      notify("error", err.message);
      document.getElementById("login-password").value = "";
      document.getElementById("login-password").focus();
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

async function start() {
  bindLogin();
  bindNewProductForm();
  bindOrders();
  bindVatForm();
  bindOutbox();
  setSignedOutHandler(signedOut);

  for (const button of document.querySelectorAll("#tabs button")) {
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
