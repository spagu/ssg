// The basket page.
//
// Rendered from localStorage, with every value written through textContent and
// element properties. Nothing here builds markup from a string, because a
// product name came from the shop's own database but a quantity came from the
// URL, and one innerHTML is all it takes to stop being able to tell them apart.

import { cart, formatMoney } from "./cart.js";

function cell(label, ...children) {
  const td = document.createElement("td");
  td.dataset.label = label;
  td.append(...children);
  return td;
}

function lineRow(line, onChange) {
  const row = document.createElement("tr");

  const item = document.createElement("div");
  item.className = "ps-cart-item";
  if (line.image) {
    const img = document.createElement("img");
    img.className = "ps-cart-thumb";
    img.src = line.image;
    img.alt = "";
    img.loading = "lazy";
    item.append(img);
  }
  const link = document.createElement("a");
  link.href = line.url || "#";
  link.textContent = line.name;
  item.append(link);
  row.append(cell("Item", item));

  row.append(cell("Price", document.createTextNode(formatMoney(line.priceMinor, line.currency))));

  const qty = document.createElement("input");
  qty.type = "number";
  qty.className = "ps-qty";
  qty.min = "1";
  qty.max = "10";
  qty.step = "1";
  qty.value = String(line.quantity);
  qty.setAttribute("aria-label", `Quantity of ${line.name}`);
  qty.addEventListener("change", () => {
    cart.setQuantity(line.sku, qty.value);
    onChange();
  });
  row.append(cell("Quantity", qty));

  row.append(
    cell("Total", document.createTextNode(formatMoney(line.priceMinor * line.quantity, line.currency))),
  );

  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "ps-remove";
  remove.textContent = "Remove";
  remove.setAttribute("aria-label", `Remove ${line.name} from the basket`);
  remove.addEventListener("click", () => {
    cart.remove(line.sku);
    onChange();
  });
  row.append(cell("", remove));

  return row;
}

export function mountCart() {
  const root = document.querySelector("[data-cart-page]");
  if (!root) return;

  const body = root.querySelector("[data-cart-rows]");
  const empty = root.querySelector("[data-cart-empty]");
  const table = root.querySelector("[data-cart-table]");
  const totalOut = root.querySelector("[data-cart-total]");
  const actions = root.querySelector("[data-cart-actions]");
  const warning = root.querySelector("[data-cart-warning]");

  const render = () => {
    const lines = cart.lines();
    body.replaceChildren(...lines.map((line) => lineRow(line, render)));

    const isEmpty = lines.length === 0;
    if (empty) empty.hidden = !isEmpty;
    if (table) table.hidden = isEmpty;
    if (actions) actions.hidden = isEmpty;

    const total = cart.total();
    if (totalOut) {
      totalOut.textContent = total.mixed ? "—" : formatMoney(total.amountMinor, total.currency);
    }
    // Two products priced in different currencies cannot be one payment. Say
    // so here rather than letting the checkout refuse it later.
    if (warning) warning.hidden = !total.mixed;
  };

  render();
  document.addEventListener("picostore:cart", render);

  root.querySelector("[data-cart-clear]")?.addEventListener("click", () => {
    cart.clear();
    render();
  });
}
