// Settings, the modules, and the VAT rate table.

import { api } from "./api.js";
import { badge, confirmDestructive, el, notify, td, when } from "./dom.js";

/** A settings key, as the panel presents it.
 *
 *  `help` is the sentence that stops a seller guessing; `type` is what the key
 *  actually is, which matters because three of them are enumerations that the
 *  API rejects on a typo and which therefore have no business being text
 *  boxes. */
const FIELDS = {
  "seller.name": { label: "Name or company", help: "Goes on every invoice.", width: "" },
  "seller.address": { label: "Address", help: "One line per line, as it should print.", type: "textarea" },
  "seller.country": { label: "Country", help: "Two letters. Decides your own VAT position.", width: "narrow" },
  "seller.vat_id": { label: "VAT number", help: "Leave empty if you are not registered — the shop then charges none.", width: "medium" },
  "seller.email": { label: "Contact email", help: "Printed on invoices; where buyers write.", type: "email" },
  "seller.registry": { label: "Registry entry", help: "Company or court register number, if your law wants it on the invoice." },

  "shop.name": { label: "Shop name", help: "Appears in emails and on the payment page." },
  "shop.url": { label: "Shop address", help: "https://… Used in emails when no request can say.", type: "url" },
  "shop.countries_allowed": {
    label: "Countries you sell to",
    help: "Comma-separated codes. Empty means everywhere the VAT table covers.",
  },

  "currency.base": { label: "Currency", help: "Three letters. Prices are entered in this.", width: "narrow" },
  "pricing.mode": {
    label: "Prices are",
    help: "Gross: the shown price includes tax and the share differs by country. Net: tax is added at checkout.",
    options: [["gross", "tax included"], ["net", "tax added at checkout"]],
    width: "medium",
  },
  "tax.mode": {
    label: "Tax handling",
    help: "Table: this shop's own rates. Stripe: let Stripe Tax decide. None: charge no tax at all.",
    options: [["table", "this shop's rate table"], ["stripe", "Stripe Tax"], ["none", "no tax"]],
    width: "medium",
  },
  "tax.rounding": {
    label: "Rounding",
    help: "Half up, which is what a tax authority means by rounded.",
    options: [["half_up", "half up"]],
    width: "medium",
  },

  "invoice.format": { label: "Number format", help: "{series}, {year} and {n:06} are filled in. Must contain {n}, and {series} unless credit notes are to collide with invoices.", width: "medium" },
  "invoice.series": { label: "Invoice series", help: "The prefix for ordinary invoices.", width: "narrow" },
  "invoice.credit_series": { label: "Credit note series", help: "The prefix for corrections.", width: "narrow" },
  "invoice.separate_email": { label: "Send the invoice in its own email", help: "Otherwise it goes with the download links.", type: "check" },
  "invoice.vat_in_local": { label: "Show VAT in your own currency", help: "Some tax offices want this on a cross-currency invoice.", type: "check" },

  "downloads.renew_on_resend": {
    label: "Issue fresh links when a buyer asks",
    help: "Someone who lost the email a month later gets working links rather than nothing.",
    type: "check",
  },

  "legal.digital_waiver": {
    label: "Ask for the withdrawal waiver",
    help: "Required to deliver immediately in the EU and the UK. Without it you would have to wait out the 14 days.",
    type: "check",
  },
  "legal.terms_url": { label: "Terms page", help: "Linked from checkout and from emails.", type: "url" },
  "legal.privacy_url": { label: "Privacy page", help: "Linked from checkout and from emails.", type: "url" },

  "webhooks.endpoints": {
    label: "Endpoints",
    help: 'JSON: [{"url":"https://…","events":["order.paid"],"secret":"…"}]. An endpoint naming no events receives none.',
    type: "json",
  },
};

/** The screen, in the order a seller fills it in. */
const SECTIONS = [
  {
    title: "Who is selling",
    blurb: "Frozen into every invoice at the moment it is issued, so changing it later does not rewrite history.",
    keys: ["seller.name", "seller.address", "seller.country", "seller.vat_id", "seller.email", "seller.registry"],
  },
  {
    title: "The shop",
    blurb: "How it introduces itself, and where it will sell.",
    keys: ["shop.name", "shop.url", "shop.countries_allowed"],
  },
  {
    title: "Money and tax",
    blurb: "The two decisions everything else follows from.",
    keys: ["currency.base", "pricing.mode", "tax.mode", "tax.rounding"],
  },
  {
    title: "Invoices",
    blurb: "Numbering runs without gaps and cannot be edited afterwards; a correction is a credit note.",
    keys: ["invoice.format", "invoice.series", "invoice.credit_series", "invoice.separate_email", "invoice.vat_in_local"],
  },
  { title: "Delivery", blurb: "", keys: ["downloads.renew_on_resend"] },
  {
    title: "Legal",
    blurb: "Both pages are linked from the checkout, so both had better exist.",
    keys: ["legal.terms_url", "legal.privacy_url", "legal.digital_waiver"],
  },
  {
    title: "Outgoing webhooks",
    blurb: "Signed the way Stripe signs its own, so a recipient that already integrates Stripe knows the format.",
    keys: ["webhooks.endpoints"],
  },
];

export async function loadSettings() {
  const { settings, keys } = await api.get("/api/shop/admin/settings");
  const form = document.getElementById("settings-form");

  // Anything declared by the API and not placed in a section still has to be
  // editable: a new setting must not be invisible until someone updates this
  // file.
  const placed = new Set(SECTIONS.flatMap((s) => s.keys));
  const strays = keys.filter((k) => !placed.has(k) && !k.startsWith("modules."));
  const sections = strays.length
    ? [...SECTIONS, { title: "Other", blurb: "Declared by this shop's API and not yet given a home here.", keys: strays }]
    : SECTIONS;

  form.replaceChildren(
    ...sections.map((section) => renderSection(section, settings)),
    el("div", { class: "savebar" },
      el("p", { text: "Saved settings apply within a few seconds. Nothing here is a secret — those live in wrangler." }),
      el("span", { class: "spacer" }),
      el("button", { type: "submit" }, "Save settings"),
    ),
  );

  form.onsubmit = async (event) => {
    event.preventDefault();
    const patch = {};
    for (const key of keys) {
      if (key.startsWith("modules.")) continue; // the Modules screen owns those
      const control = form.querySelector(`[name="${CSS.escape(key)}"]`);
      if (!control) continue;
      patch[key] = readControl(key, control, settings[key]);
    }
    try {
      await api.put("/api/shop/admin/settings", patch);
      notify("ok", "Settings saved.");
    } catch (err) {
      notify("error", err.message);
    }
  };
}

function renderSection(section, settings) {
  const checks = section.keys.filter((k) => FIELDS[k]?.type === "check");
  const inputs = section.keys.filter((k) => FIELDS[k]?.type !== "check");

  return el("div", { class: "card" },
    el("header", {},
      el("div", {},
        el("h3", { text: section.title }),
        section.blurb ? el("p", { text: section.blurb }) : null,
      ),
    ),
    el("div", { class: "card-body" },
      inputs.length
        ? el("div", { class: "fields two" }, ...inputs.map((key) => renderField(key, settings[key])))
        : null,
      checks.length ? el("div", { class: "fields" }, ...checks.map((key) => renderCheck(key, settings[key]))) : null,
    ),
  );
}

function renderField(key, value) {
  const spec = FIELDS[key] ?? { label: key };
  const control = controlFor(key, value, spec);
  control.id = `s-${key.replace(/\W+/g, "-")}`;
  return el("div", { class: `field ${spec.width ?? ""}`.trim() },
    el("label", { for: control.id, text: spec.label }),
    control,
    spec.help ? el("small", { text: spec.help }) : null,
  );
}

function renderCheck(key, value) {
  const spec = FIELDS[key] ?? { label: key };
  const input = el("input", {
    id: `s-${key.replace(/\W+/g, "-")}`,
    name: key,
    type: "checkbox",
    checked: Boolean(value),
  });
  return el("div", { class: "check" },
    input,
    el("label", { for: input.id, text: spec.label }),
    spec.help ? el("small", { text: spec.help }) : null,
  );
}

function controlFor(key, value, spec) {
  if (spec.options) {
    return el("select", { name: key },
      ...spec.options.map(([v, label]) => el("option", { value: v, selected: v === value }, label)),
    );
  }
  if (spec.type === "textarea") return el("textarea", { name: key }, String(value ?? ""));
  if (spec.type === "json") {
    return el("textarea", { name: key, "data-shape": "json", spellcheck: "false" },
      JSON.stringify(Array.isArray(value) ? value : [], null, 2));
  }
  if (Array.isArray(value)) {
    // A list of country codes is comfortable as text; a list of objects is not.
    return el("input", { name: key, "data-shape": "csv", value: value.join(", ") });
  }
  return el("input", { name: key, type: spec.type ?? "text", value: String(value ?? ""), maxlength: "500" });
}

function readControl(key, control, previous) {
  if (control.type === "checkbox") return control.checked;
  if (control.dataset.shape === "csv") {
    return control.value.split(",").map((s) => s.trim()).filter(Boolean);
  }
  if (control.dataset.shape === "json") {
    try {
      return JSON.parse(control.value || "[]");
    } catch {
      // Unparseable JSON must not be sent as a string: it would be stored and
      // read back as something nothing understands. The old value stands, and
      // the message says why.
      notify("error", `${key} is not valid JSON, so it was left as it was.`);
      return previous;
    }
  }
  return control.value;
}

// ── Modules ─────────────────────────────────────────────────────────────────

/** What each module is, in a sentence a seller can act on. */
const MODULES = {
  invoices: {
    title: "Invoices",
    text: "Issue a numbered invoice for every paid order, and a credit note for every refund. Off if your accountant issues the paperwork elsewhere — orders are still paid and still delivered, they simply have no invoice of ours.",
  },
  emails: {
    title: "Buyer emails",
    text: "The “here is your book” email, with the download links. With this off the buyer is left with the thank-you page and nothing in their inbox.",
  },
  admin_notices: {
    title: "Owner notices",
    text: "A line to you when something sells or needs a decision. It carries the order number and the amount, never the buyer's details.",
  },
  webhooks: {
    title: "Outgoing webhooks",
    text: "Tell your own systems when an order is paid, refunded or flagged. Signed the way Stripe signs its own.",
  },
  tracking: {
    title: "Server-side tracking",
    text: "Send the purchase to GA4 or Meta from the server, after the payment clears — no third-party script on your pages. Only ever for a buyer who ticked the marketing box.",
  },
  turnstile: {
    title: "Anti-spam check",
    text: "Turnstile on checkout and on the “lost my download” form. Switch it off to test a storefront without solving a challenge each time.",
  },
  resend: {
    title: "Public “lost my download”",
    text: "The form a buyer uses to have their links sent again. With it off the endpoint does not exist, and the theme stops linking to it.",
  },
};

export async function loadModules() {
  const { modules, settings } = await api.get("/api/shop/admin/settings");
  const host = document.getElementById("module-rows");

  host.replaceChildren(...modules.map((m) => moduleRow(m, settings)));
}

function moduleRow(state, settings) {
  const spec = MODULES[state.name] ?? { title: state.name, text: "" };
  const blocked = Boolean(state.blockedBy);

  const input = el("input", {
    type: "checkbox",
    id: `m-${state.name}`,
    checked: state.on && !blocked,
    // A switch offered for something with nothing behind it is a switch that
    // lies: it is shown, disabled, beside the reason.
    disabled: blocked,
    onChange: () => toggle(state.name, input, settings),
  });

  return el("div", { class: "module" },
    el("h4", {},
      el("label", { for: input.id, text: spec.title }),
      // Three states, not two: a module can be switched off, or switched on and
      // unable to run. Calling the second one "off" would blame the seller for
      // a missing secret.
      blocked
        ? el("span", { class: "badge", "data-status": "needs_review", text: "unavailable" })
        : badge(state.on ? "on" : "off"),
    ),
    el("p", {},
      spec.text,
      blocked ? el("span", { class: "blocked", text: ` Needs ${state.blockedBy}.` }) : null,
    ),
    el("div", { class: "switch" }, input, el("span", { "aria-hidden": "true" })),
  );
}

async function toggle(name, input, settings) {
  const wanted = input.checked;
  try {
    await api.put("/api/shop/admin/settings", { [`modules.${name}`]: wanted });
    settings[`modules.${name}`] = wanted;
    notify("ok", `${MODULES[name]?.title ?? name} switched ${wanted ? "on" : "off"}.`);
    await loadModules();
  } catch (err) {
    // Put the switch back where it was: it shows what the shop does, and the
    // shop did not change.
    input.checked = !wanted;
    notify("error", err.message);
  }
}

// ── VAT ─────────────────────────────────────────────────────────────────────

export async function loadVat() {
  const { rates } = await api.get("/api/shop/admin/vat-rates");
  const rows = document.getElementById("vat-rows");

  rows.replaceChildren(
    ...rates.map((r) =>
      el("tr", {},
        td("Country", el("strong", { text: r.country })),
        td("Kind", r.kind),
        td("Rate", { class: "numeric" }, `${(r.rate_bp / 100).toFixed(2)}%`),
        td("In force", r.valid_to
          ? `${when(r.valid_from)} — ${when(r.valid_to)}`
          : el("span", {}, `${when(r.valid_from)} — `, el("strong", { text: "now" }))),
        td("", { class: "actions" },
          el("button", { type: "button", class: "ghost", onClick: () => removeRate(r) }, "Remove"),
        ),
      ),
    ),
  );
  if (rates.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "5", class: "muted", text: "No rates. The shop cannot price a taxable sale." })));
  }
}

async function removeRate(rate) {
  if (!confirmDestructive(`Remove the ${rate.kind} rate for ${rate.country} valid from ${rate.valid_from}?`)) return;
  const params = new URLSearchParams({ country: rate.country, kind: rate.kind, validFrom: rate.valid_from });
  try {
    await api.del(`/api/shop/admin/vat-rates?${params}`);
    notify("ok", "Rate removed.");
    await loadVat();
  } catch (err) {
    notify("error", err.message);
  }
}

export function bindVatForm() {
  const form = document.getElementById("vat-new");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const validFrom = String(data.get("validFrom") ?? "");
    try {
      await api.post("/api/shop/admin/vat-rates", {
        country: data.get("country"),
        kind: data.get("kind"),
        rateBp: Number(data.get("rateBp")),
        // A date alone means the start of that day, which is how a rate change
        // is published.
        validFrom: validFrom ? `${validFrom}T00:00:00.000Z` : undefined,
      });
      notify("ok", "Rate set. The one it replaces is closed off at that date.");
      await loadVat();
    } catch (err) {
      notify("error", err.message);
    }
  });
}
