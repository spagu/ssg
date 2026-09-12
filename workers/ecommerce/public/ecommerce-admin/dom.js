// Building elements without building HTML.
//
// Every value that reaches the page is set through textContent or a property,
// never through innerHTML. Product names, customer names and error messages
// from payment providers are all text somebody else wrote; the panel is the one
// place where the shop's own staff would be the victim of getting that wrong.

/** el("td", {class: "numeric"}, "19.00") */
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue;
    if (key === "class") node.className = value;
    else if (key === "text") node.textContent = value;
    else if (key.startsWith("on") && typeof value === "function") {
      node.addEventListener(key.slice(2).toLowerCase(), value);
    } else if (key === "dataset") Object.assign(node.dataset, value);
    else node.setAttribute(key, value === true ? "" : String(value));
  }
  for (const child of children.flat()) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}

/** A table cell carrying its own column name, which is what makes the table
 *  readable when it stacks on a narrow screen. */
export function td(label, ...children) {
  return el("td", { "data-label": label }, ...children);
}

export function badge(status) {
  return el("span", { class: "badge", "data-status": status, text: String(status).replace("_", " ") });
}

export function money(amountMinor, currency) {
  if (amountMinor === null || amountMinor === undefined || !currency) return "—";
  const zero = ["JPY", "KRW", "VND", "CLP", "XAF", "XOF", "XPF", "BIF", "DJF", "GNF", "KMF", "MGA", "PYG", "RWF", "UGX", "VUV"];
  const three = ["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"];
  const digits = zero.includes(currency) ? 0 : three.includes(currency) ? 3 : 2;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(amountMinor / 10 ** digits);
  } catch {
    return `${(amountMinor / 10 ** digits).toFixed(digits)} ${currency}`;
  }
}

export function when(iso) {
  if (!iso) return "—";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString();
}

/** One place for every message the panel shows, so an error cannot be missed
 *  because it was drawn somewhere the eye was not. */
export function notify(kind, message) {
  const box = document.getElementById("notice");
  box.textContent = message;
  box.dataset.kind = kind;
  box.hidden = false;
  box.setAttribute("role", kind === "error" ? "alert" : "status");
  if (kind !== "error") setTimeout(() => (box.hidden = true), 5000);
}

export function clearNotice() {
  document.getElementById("notice").hidden = true;
}

/** Confirms a step that cannot be taken back. Deliberately the browser's own
 *  dialog: a custom one that can be styled is a custom one that can be
 *  mis-styled into something nobody reads. */
export function confirmDestructive(question) {
  return window.confirm(question);
}
