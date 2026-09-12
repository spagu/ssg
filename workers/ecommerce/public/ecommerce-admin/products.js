// The products screen: the list, and the editor that opens under it.

import { api } from "./api.js";
import { badge, confirmDestructive, el, money, notify, td } from "./dom.js";

let products = [];
let baseCurrency = "EUR";

export function setBaseCurrency(currency) {
  if (currency) baseCurrency = currency;
}

export async function loadProducts() {
  const body = await api.get("/api/shop/admin/products");
  products = body.products ?? [];
  render();
}

function render() {
  const rows = document.getElementById("product-rows");

  if (products.length === 0) {
    rows.replaceChildren(
      el("tr", {},
        el("td", { colspan: "5" },
          el("div", { class: "empty" },
            el("h3", { text: "Nothing to sell yet" }),
            el("p", { text: "Add a product, give it a price and upload the file buyers receive. It goes on sale when it has all three." }),
            el("button", { type: "button", onClick: () => openNewProduct() }, "Add product"),
          ),
        ),
      ),
    );
    return;
  }

  rows.replaceChildren(
    ...products.map((p) =>
      el(
        "tr",
        {},
        td("Product",
          el("div", { class: "cell-media" },
            p.image_url
              ? el("img", { class: "thumb", src: p.image_url, alt: "", loading: "lazy" })
              : el("span", { class: "thumb" }),
            el("div", { class: "cell-title" },
              el("strong", { text: p.name }),
              el("small", {}, el("code", { text: p.sku })),
            ),
          ),
        ),
        td("Status", badge(p.status)),
        td("Price", { class: "numeric" },
          p.prices?.[baseCurrency] === undefined
            ? el("span", { class: "muted", text: "no price" })
            : money(p.prices[baseCurrency], baseCurrency),
        ),
        // A product with no file cannot be sold, and this column is where the
        // owner finds that out — not the buyer.
        td("File",
          p.file_name
            ? el("span", { class: "muted", text: `${p.file_name} · ${Math.max(1, Math.round((p.file_size ?? 0) / 1024))} kB` })
            : el("span", { class: "muted", text: "none yet" }),
        ),
        td("", { class: "actions" },
          el("button", { type: "button", class: "secondary small", onClick: () => openEditor(p.id) }, "Edit"),
        ),
      ),
    ),
  );
}

// ── Creating one ────────────────────────────────────────────────────────────

function openNewProduct() {
  const card = document.getElementById("product-new-card");
  card.hidden = false;
  document.getElementById("new-sku").focus();
}

export function bindNewProductForm() {
  const card = document.getElementById("product-new-card");
  const form = document.getElementById("product-new");

  document.getElementById("product-add").addEventListener("click", openNewProduct);
  document.getElementById("product-new-cancel").addEventListener("click", () => {
    form.reset();
    card.hidden = true;
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      const { product } = await api.post("/api/shop/admin/products", {
        sku: data.get("sku"),
        name: data.get("name"),
      });
      form.reset();
      card.hidden = true;
      notify("ok", "Product created. Give it a price and a file before putting it on sale.");
      await loadProducts();
      // Straight into the editor: creating a product is never the whole job.
      await openEditor(product.id);
    } catch (err) {
      notify("error", err.message);
    }
  });
}

// ── Editing one ─────────────────────────────────────────────────────────────

/** Back to the list, from wherever the detail page was opened. */
function closeEditor() {
  document.getElementById("product-editor").hidden = true;
  document.getElementById("products-list").hidden = false;
  document.getElementById("crumb").textContent = "Products";
  window.scrollTo({ top: 0 });
}

async function openEditor(id) {
  const panel = document.getElementById("product-editor");
  const { product, soldCount } = await api.get(`/api/shop/admin/products/${encodeURIComponent(id)}`);

  const form = el(
    "form",
    {
      onSubmit: async (event) => {
        event.preventDefault();
        const data = new FormData(form);
        const price = String(data.get("price") ?? "").trim();
        try {
          await api.patch(`/api/shop/admin/products/${encodeURIComponent(id)}`, {
            name: data.get("name"),
            description: data.get("description"),
            status: data.get("status"),
            taxCategory: data.get("taxCategory"),
            downloadLimit: Number(data.get("downloadLimit")),
            downloadDays: Number(data.get("downloadDays")),
            imageUrl: data.get("imageUrl"),
            // Typed in whole currency units, stored in minor ones. The panel
            // does the multiplication so nobody has to think in cents.
            prices: price === "" ? undefined : { [baseCurrency]: toMinor(price, baseCurrency) },
          });
          notify("ok", "Saved.");
          await loadProducts();
          closeEditor();
        } catch (err) {
          notify("error", err.message);
        }
      },
    },
    el("div", { class: "fields two" },
      field("Name", el("input", { name: "name", value: product.name, maxlength: "200", required: true })),
      field("Product code", el("input", { value: product.sku, disabled: true }),
        "Fixed once created: it is on the invoices already issued."),
    ),
    field("Description", el("textarea", { name: "description", maxlength: "20000" }, product.description ?? ""),
      "Shown in the catalogue API. The storefront decides whether to use it."),
    el("div", { class: "fields two" },
      field(
        `Price (${baseCurrency})`,
        el("input", {
          name: "price",
          type: "text",
          inputmode: "decimal",
          value: product.prices?.[baseCurrency] !== undefined ? fromMinor(product.prices[baseCurrency], baseCurrency) : "",
          placeholder: "19.00",
        }),
        "Whole units. Leave empty to keep the current price.",
      ),
      field("Status", select("status", ["draft", "active", "archived"], product.status),
        "Only active products appear in the catalogue."),
      field("Tax category", el("input", { name: "taxCategory", value: product.tax_category, maxlength: "32" }),
        "Which VAT rate to look up. Falls back to the standard rate."),
    ),
    el("div", { class: "fields two" },
      field("Downloads allowed", el("input", { name: "downloadLimit", type: "number", min: "1", max: "100", value: String(product.download_limit) }),
        "Per order. A ranged continuation does not count again."),
      field("Link valid for", el("input", { name: "downloadDays", type: "number", min: "1", max: "3650", value: String(product.download_days) }),
        "Days. A buyer can always ask for a fresh link."),
      field("Cover image URL", el("input", { name: "imageUrl", value: product.image_url ?? "", maxlength: "500" })),
    ),
    el("div", { class: "savebar" },
      el("p", { text: soldCount > 0
        ? `Sold ${soldCount} time${soldCount === 1 ? "" : "s"}. It can be archived but not deleted: its orders and invoices refer to it.`
        : "Not sold yet, so it can still be deleted outright." }),
      el("span", { class: "spacer" }),
      el("button", {
        type: "button",
        class: "danger",
        onClick: () => removeProduct(id, product.sku, soldCount),
      }, soldCount > 0 ? "Archive" : "Delete"),
      el("button", { type: "button", class: "secondary", onClick: closeEditor }, "Discard"),
      el("button", { type: "submit" }, "Save"),
    ),
  );

  panel.replaceChildren(
    // A page of its own, with the way back where a reader looks for it.
    el("div", { class: "page-head" },
      el("div", {},
        el("button", { type: "button", class: "ghost back", onClick: closeEditor },
          el("span", { "aria-hidden": "true", text: "←" }), " Products"),
        el("h2", { text: product.name }),
        el("p", {}, "Product code ", el("code", { text: product.sku }), " · ",
          soldCount > 0 ? `sold ${soldCount} time${soldCount === 1 ? "" : "s"}` : "not sold yet"),
      ),
      el("div", { class: "actions" }, badge(product.status)),
    ),
    el("div", { class: "card" },
      el("header", {}, el("div", {}, el("h3", { text: "Details" }))),
      el("div", { class: "card-body" }, form),
    ),
    uploadCard(id, product),
  );

  document.getElementById("products-list").hidden = true;
  panel.hidden = false;
  document.getElementById("crumb").textContent = product.name;
  window.scrollTo({ top: 0 });
}

function uploadCard(id, product) {
  const status = el("p", { class: "muted", text: product.file_name
    ? `${product.file_name} · ${Math.max(1, Math.round((product.file_size ?? 0) / 1024))} kB · SHA-256 ${String(product.file_sha256 ?? "").slice(0, 16)}…`
    : "No file yet. A product cannot go on sale without one." });

  const input = el("input", { id: `file-${id}`, type: "file", accept: ".pdf,.epub,.mobi,.azw3" });
  const form = el(
    "form",
    {
      onSubmit: async (event) => {
        event.preventDefault();
        if (!input.files?.[0]) return;
        const data = new FormData();
        data.append("file", input.files[0]);
        try {
          const result = await api.upload(`/api/shop/admin/products/${encodeURIComponent(id)}/file`, data);
          status.textContent = `${result.fileName} · ${result.contentType} · SHA-256 ${result.sha256.slice(0, 16)}…`;
          notify("ok", "File stored. Buyers of this product get this file from now on.");
          await loadProducts();
        } catch (err) {
          notify("error", err.message);
        }
      },
    },
    el("div", { class: "field" },
      el("label", { for: input.id, text: "Choose a file" }),
      input,
      el("small", { text: "PDF, EPUB or MOBI. Checked against its own first bytes, not its extension." }),
    ),
    el("div", { class: "savebar" },
      el("span", { class: "spacer" }),
      el("button", { type: "submit" }, "Upload"),
    ),
  );

  return el("div", { class: "card" },
    el("header", {},
      el("div", {},
        el("h3", { text: "The file buyers receive" }),
        el("p", { text: "Replacing it leaves the old one in place for orders whose links still work." }),
      ),
    ),
    el("div", { class: "card-body" }, status, form),
  );
}

async function removeProduct(id, sku, soldCount) {
  const question = soldCount > 0
    ? `Archive ${sku}? It stays on its existing orders and stops being for sale.`
    : `Delete ${sku} for good? This also deletes its file.`;
  if (!confirmDestructive(question)) return;
  try {
    const result = await api.del(`/api/shop/admin/products/${encodeURIComponent(id)}`);
    notify("ok", result.deleted ? "Deleted." : "Archived.");
    await loadProducts();
    closeEditor();
  } catch (err) {
    notify("error", err.message);
  }
}

// ── Small builders ──────────────────────────────────────────────────────────

let fieldSeq = 0;

function field(label, control, help) {
  control.id = control.id || `f-${++fieldSeq}`;
  return el("div", { class: "field" },
    el("label", { for: control.id, text: label }),
    control,
    help ? el("small", { text: help }) : null,
  );
}

function select(name, options, current) {
  return el(
    "select",
    { name },
    ...options.map((value) => el("option", { value, selected: value === current }, value)),
  );
}

function decimalsFor(currency) {
  const zero = ["JPY", "KRW", "VND", "CLP", "XAF", "XOF", "XPF", "BIF", "DJF", "GNF", "KMF", "MGA", "PYG", "RWF", "UGX", "VUV"];
  const three = ["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"];
  return zero.includes(currency) ? 0 : three.includes(currency) ? 3 : 2;
}

function toMinor(value, currency) {
  const digits = decimalsFor(currency);
  return Math.round(Number(value.replace(",", ".")) * 10 ** digits);
}

function fromMinor(minor, currency) {
  const digits = decimalsFor(currency);
  return (minor / 10 ** digits).toFixed(digits);
}
