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
  rows.replaceChildren(
    ...products.map((p) =>
      el(
        "tr",
        {},
        td("Code", el("code", { text: p.sku })),
        td("Name", p.name),
        td("Status", badge(p.status)),
        td("Price", money(p.prices?.[baseCurrency], baseCurrency)),
        // A product with no file cannot be sold, and this column is where the
        // owner finds that out — not the buyer.
        td("File", p.file_name ? `${p.file_name} (${Math.round((p.file_size ?? 0) / 1024)} kB)` : "none yet"),
        td("Edit", el("button", { class: "quiet", type: "button", onClick: () => openEditor(p.id) }, "Edit")),
      ),
    ),
  );
  if (products.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "6", text: "No products yet." })));
  }
}

export function bindNewProductForm() {
  const form = document.getElementById("product-new");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      await api.post("/api/shop/admin/products", {
        sku: data.get("sku"),
        name: data.get("name"),
      });
      form.reset();
      notify("ok", "Product created. Add a price and a file before putting it on sale.");
      await loadProducts();
    } catch (err) {
      notify("error", err.message);
    }
  });
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
        const price = data.get("price").toString().trim();
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
          panel.hidden = true;
        } catch (err) {
          notify("error", err.message);
        }
      },
    },
    el("div", { class: "row" },
      field("Name", el("input", { name: "name", value: product.name, maxlength: "200", required: true })),
      field("Code", el("input", { value: product.sku, disabled: true })),
    ),
    field("Description", el("textarea", { name: "description", maxlength: "20000" }, product.description ?? "")),
    el("div", { class: "row" },
      field(
        `Price (${baseCurrency})`,
        el("input", {
          name: "price",
          type: "text",
          inputmode: "decimal",
          value: product.prices?.[baseCurrency] !== undefined ? fromMinor(product.prices[baseCurrency], baseCurrency) : "",
          placeholder: "19.00",
        }),
      ),
      field("Status", select("status", ["draft", "active", "archived"], product.status)),
      field("Tax category", el("input", { name: "taxCategory", value: product.tax_category, maxlength: "32" })),
    ),
    el("div", { class: "row" },
      field("Downloads allowed", el("input", { name: "downloadLimit", type: "number", min: "1", max: "100", value: String(product.download_limit) })),
      field("Link valid for (days)", el("input", { name: "downloadDays", type: "number", min: "1", max: "3650", value: String(product.download_days) })),
      field("Cover image URL", el("input", { name: "imageUrl", value: product.image_url ?? "", maxlength: "500" })),
    ),
    el("div", { class: "row" },
      el("button", { type: "submit" }, "Save"),
      el("button", { type: "button", class: "quiet", onClick: () => (panel.hidden = true) }, "Close"),
      el("button", {
        type: "button",
        class: "danger",
        onClick: () => removeProduct(id, product.sku, soldCount, panel),
      }, soldCount > 0 ? "Archive" : "Delete"),
    ),
  );

  panel.replaceChildren(
    el("div", { class: "editor" },
      el("h3", { text: product.name }),
      el("p", { class: "hint", text: soldCount > 0
        ? `Sold ${soldCount} time(s). It can be archived but not deleted: its orders and invoices refer to it.`
        : "Not sold yet, so it can still be deleted outright." }),
      form,
      uploadForm(id, product),
    ),
  );
  panel.hidden = false;
  panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

function uploadForm(id, product) {
  const status = el("p", { class: "hint", text: product.file_name
    ? `Current file: ${product.file_name}, SHA-256 ${String(product.file_sha256 ?? "").slice(0, 16)}…`
    : "No file yet. A product cannot go on sale without one." });

  const form = el(
    "form",
    {
      onSubmit: async (event) => {
        event.preventDefault();
        const input = form.querySelector("input[type=file]");
        if (!input.files?.[0]) return;
        const data = new FormData();
        data.append("file", input.files[0]);
        try {
          const result = await api.upload(`/api/shop/admin/products/${encodeURIComponent(id)}/file`, data);
          status.textContent = `Uploaded ${result.fileName} (${result.contentType}), SHA-256 ${result.sha256.slice(0, 16)}…`;
          notify("ok", "File stored. Buyers of this product get this file from now on.");
          await loadProducts();
        } catch (err) {
          notify("error", err.message);
        }
      },
    },
    el("label", { for: `file-${id}`, text: "Replace the file (PDF, EPUB or MOBI)" }),
    el("input", { id: `file-${id}`, type: "file", accept: ".pdf,.epub,.mobi,.azw3" }),
    el("button", { type: "submit" }, "Upload"),
  );
  return el("div", {}, el("h3", { text: "The file buyers receive" }), status, form);
}

async function removeProduct(id, sku, soldCount, panel) {
  const question = soldCount > 0
    ? `Archive ${sku}? It stays on its existing orders and stops being for sale.`
    : `Delete ${sku} for good? This also deletes its file.`;
  if (!confirmDestructive(question)) return;
  try {
    const result = await api.del(`/api/shop/admin/products/${encodeURIComponent(id)}`);
    notify("ok", result.deleted ? "Deleted." : "Archived.");
    panel.hidden = true;
    await loadProducts();
  } catch (err) {
    notify("error", err.message);
  }
}

function field(label, control) {
  const id = `f-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  control.id = control.id || id;
  return el("div", {}, el("label", { for: control.id, text: label }), control);
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
