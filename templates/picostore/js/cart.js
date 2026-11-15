// The basket: one small module the other shop pages build on.
//
// It lives in localStorage, so it survives a reload and never reaches the
// server until checkout. What it holds is a shopping list — codes and
// quantities — plus a cached name and price for drawing the page. The prices
// are for display only: the checkout endpoint prices the order again from its
// own database, so editing the number in devtools changes what you see and not
// what you pay.

const KEY = "picostore.cart.v1";
const MAX_LINES = 20;
const MAX_QTY = 10;

/** Storage can be unavailable (private window, blocked cookies) and reads can
 *  throw. A shop that breaks entirely in that case is worse than one that
 *  forgets the basket between pages. */
function read() {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(isLine) : [];
  } catch {
    return [];
  }
}

function write(lines) {
  try {
    localStorage.setItem(KEY, JSON.stringify(lines));
  } catch {
    /* nothing more we can do; the page still works for this visit */
  }
  document.dispatchEvent(new CustomEvent("picostore:cart", { detail: { lines } }));
}

function isLine(line) {
  return (
    line &&
    typeof line.sku === "string" &&
    /^[A-Za-z0-9._-]{1,64}$/.test(line.sku) &&
    Number.isFinite(line.quantity) &&
    line.quantity > 0
  );
}

export const cart = {
  lines: read,

  count() {
    return read().reduce((n, line) => n + line.quantity, 0);
  },

  /** Adds, or raises the quantity of a line already there. */
  add(item, quantity = 1) {
    const lines = read();
    const existing = lines.find((l) => l.sku === item.sku);
    if (existing) {
      existing.quantity = Math.min(MAX_QTY, existing.quantity + quantity);
    } else {
      if (lines.length >= MAX_LINES) return { ok: false, reason: "full" };
      lines.push({
        sku: item.sku,
        quantity: Math.min(MAX_QTY, Math.max(1, quantity)),
        name: String(item.name ?? item.sku).slice(0, 200),
        priceMinor: Number(item.priceMinor) || 0,
        currency: String(item.currency ?? "EUR").toUpperCase().slice(0, 3),
        url: String(item.url ?? "").slice(0, 500),
        image: String(item.image ?? "").slice(0, 500),
      });
    }
    write(lines);
    return { ok: true };
  },

  setQuantity(sku, quantity) {
    const lines = read();
    const line = lines.find((l) => l.sku === sku);
    if (!line) return;
    const wanted = Math.floor(Number(quantity));
    if (!Number.isFinite(wanted) || wanted < 1) return this.remove(sku);
    line.quantity = Math.min(MAX_QTY, wanted);
    write(lines);
  },

  remove(sku) {
    write(read().filter((l) => l.sku !== sku));
  },

  clear() {
    write([]);
  },

  /** What the checkout endpoint is sent: codes and quantities, nothing else. */
  asItems() {
    return read().map((l) => ({ sku: l.sku, quantity: l.quantity }));
  },

  /** A display total in the basket's own currency. Mixed currencies cannot be
   *  added up, so the caller is told rather than shown a wrong number. */
  total() {
    const lines = read();
    const currencies = new Set(lines.map((l) => l.currency));
    if (currencies.size > 1) return { mixed: true, currency: null, amountMinor: 0 };
    return {
      mixed: false,
      currency: lines[0]?.currency ?? null,
      amountMinor: lines.reduce((sum, l) => sum + l.priceMinor * l.quantity, 0),
    };
  },
};

/** Money for humans, in the reader's own locale. Minor units in, text out. */
export function formatMoney(amountMinor, currency) {
  if (!currency) return "";
  const zero = ["JPY", "KRW", "VND", "CLP", "XAF", "XOF", "XPF", "BIF", "DJF", "GNF", "KMF", "MGA", "PYG", "RWF", "UGX", "VUV"];
  const three = ["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"];
  const digits = zero.includes(currency) ? 0 : three.includes(currency) ? 3 : 2;
  try {
    return new Intl.NumberFormat(document.documentElement.lang || "en", {
      style: "currency",
      currency,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(amountMinor / 10 ** digits);
  } catch {
    return `${(amountMinor / 10 ** digits).toFixed(digits)} ${currency}`;
  }
}

/** Keeps the header badge honest on every page, including after a change made
 *  in another tab. */
export function bindCartBadge() {
  const badge = document.querySelector("[data-cart-count]");
  if (!badge) return;
  const paint = () => {
    const n = cart.count();
    badge.textContent = String(n);
    badge.closest("a")?.setAttribute(
      "aria-label",
      n === 1 ? "Basket, 1 item" : `Basket, ${n} items`,
    );
  };
  paint();
  document.addEventListener("picostore:cart", paint);
  window.addEventListener("storage", (e) => {
    if (e.key === KEY) paint();
  });
}
