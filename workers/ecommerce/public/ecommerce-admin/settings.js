// Settings and the VAT rate table.

import { api } from "./api.js";
import { confirmDestructive, el, notify, td, when } from "./dom.js";

/** Labels and help for the keys that are not self-explanatory. A key with no
 *  entry here is rendered from its own name. */
const FIELDS = {
  "seller.name": ["Your name or company", "Goes on every invoice."],
  "seller.address": ["Address", "One per line, as it should print."],
  "seller.country": ["Country", "Two letters. Decides your own VAT position."],
  "seller.vat_id": ["Your VAT number", "Leave empty if you are not registered."],
  "seller.email": ["Contact email", "Printed on invoices; where buyers write."],
  "seller.registry": ["Registry entry", "KRS/company number, if your law wants it on the invoice."],
  "shop.name": ["Shop name", "Appears in emails and on the payment page."],
  "shop.url": ["Shop address", "https://… — used in emails when no request tells us."],
  "shop.countries_allowed": ["Countries you sell to", "Comma-separated codes. Empty means everywhere the VAT table covers."],
  "currency.base": ["Currency", "Three letters. Prices are entered in this."],
  "pricing.mode": ["Prices are", "gross = tax included in the shown price; net = added at checkout."],
  "tax.mode": ["Tax handling", "table = this shop's own rates; stripe = let Stripe decide; none = no tax."],
  "tax.rounding": ["Rounding", "half_up, which is what tax authorities mean by rounded."],
  "invoice.format": ["Invoice number format", "{series}, {year} and {n:06} are filled in. Must contain {n}, and {series} unless credit notes are to collide with invoices."],
  "invoice.series": ["Invoice series", "The prefix for ordinary invoices."],
  "invoice.credit_series": ["Credit note series", "The prefix for corrections."],
  "invoice.separate_email": ["Invoice in its own email", "Otherwise it goes with the download links."],
  "invoice.vat_in_local": ["Show VAT in your own currency", "Some tax offices want this on cross-currency invoices."],
  "downloads.renew_on_resend": ["Renew links on request", "A buyer who lost the email gets fresh links rather than nothing."],
  "legal.digital_waiver": ["Ask for the withdrawal waiver", "Required to deliver immediately in the EU and UK."],
  "legal.terms_url": ["Terms page", "Linked from checkout and from emails."],
  "legal.privacy_url": ["Privacy page", "Linked from checkout and from emails."],
  "webhooks.endpoints": ["Outgoing webhooks", "JSON: [{\"url\":\"https://…\",\"events\":[\"order.paid\"],\"secret\":\"…\"}]"],
};

export async function loadSettings() {
  const { settings, keys } = await api.get("/api/shop/admin/settings");
  const form = document.getElementById("settings-form");

  const controls = keys.map((key) => {
    const [label, help] = FIELDS[key] ?? [key, ""];
    const value = settings[key];
    const control = controlFor(key, value);
    return el("div", {},
      el("label", { for: control.id, text: label }),
      control,
      help ? el("small", { class: "hint", text: help }) : null,
    );
  });

  form.replaceChildren(
    ...controls,
    el("button", { type: "submit" }, "Save settings"),
  );

  form.onsubmit = async (event) => {
    event.preventDefault();
    const patch = {};
    for (const key of keys) {
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

function controlFor(key, value) {
  const id = `s-${key.replace(/\W+/g, "-")}`;
  if (typeof value === "boolean") {
    return el("input", { id, name: key, type: "checkbox", checked: value });
  }
  if (Array.isArray(value)) {
    // Two shapes behind one control: a list of country codes is comfortable as
    // text, a list of webhook objects has to stay JSON.
    const isSimple = value.every((v) => typeof v === "string");
    return el("textarea", { id, name: key, "data-shape": isSimple ? "csv" : "json" },
      isSimple ? value.join(", ") : JSON.stringify(value, null, 2));
  }
  if (key === "seller.address") {
    return el("textarea", { id, name: key }, String(value ?? ""));
  }
  return el("input", { id, name: key, value: String(value ?? ""), maxlength: "500" });
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
      // then read back as something nothing understands. The old value stands
      // and the message says why.
      notify("error", `${key} is not valid JSON, so it was left as it was.`);
      return previous;
    }
  }
  return control.value;
}

// ── VAT ─────────────────────────────────────────────────────────────────────

export async function loadVat() {
  const { rates } = await api.get("/api/shop/admin/vat-rates");
  const rows = document.getElementById("vat-rows");
  rows.replaceChildren(
    ...rates.map((r) =>
      el("tr", {},
        td("Country", r.country),
        td("Kind", r.kind),
        td("Rate", `${(r.rate_bp / 100).toFixed(2)}%`),
        td("From", when(r.valid_from)),
        td("Until", r.valid_to ? when(r.valid_to) : "still current"),
        td("Remove", el("button", {
          type: "button", class: "danger",
          onClick: () => removeRate(r),
        }, "Remove")),
      ),
    ),
  );
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
      notify("ok", "Rate set. The old one is closed off at that date.");
      await loadVat();
    } catch (err) {
      notify("error", err.message);
    }
  });
}
