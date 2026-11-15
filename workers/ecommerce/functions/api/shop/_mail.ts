// Transactional email, through whatever HTTP API the seller configured.
//
// Provider-agnostic on purpose: the body is {from,to,subject,text,html}, which
// Resend, Postmark and a three-line relay all accept. MailChannels is not the
// default here, unlike the contact-form template: an email carrying the only
// link to a paid file needs a domain with SPF and DKIM behind it, and "it
// usually arrives" is not good enough when someone has paid.

import type { Env } from "./_env";
import { escapeHtml } from "./_lib";
import { formatMoney } from "./_money";

export interface MailPayload {
  subject: string;
  text: string;
  html?: string;
  replyTo?: string;
}

/** Called by the outbox. Throwing schedules a retry; returning marks it sent. */
export async function deliverEmail(env: Env, to: string, payload: Record<string, unknown>): Promise<void> {
  const url = env.SHOP_MAIL_URL;
  const from = env.SHOP_MAIL_FROM;
  if (!url || !from) {
    // Not configured is a real failure, not a silent success: the row stays in
    // the panel saying exactly what is missing.
    throw new Error("mail_not_configured: set SHOP_MAIL_URL and SHOP_MAIL_FROM");
  }
  const headers: Record<string, string> = { "content-type": "application/json" };
  if (env.SHOP_MAIL_KEY) headers.authorization = `Bearer ${env.SHOP_MAIL_KEY}`;

  const body: Record<string, unknown> = {
    from,
    to: [to],
    subject: payload.subject,
    text: payload.text,
  };
  if (payload.html) body.html = payload.html;
  if (payload.replyTo) body.reply_to = payload.replyTo;

  const res = await fetch(url, { method: "POST", headers, body: JSON.stringify(body) });
  if (!res.ok) {
    const detail = (await res.text()).slice(0, 300);
    throw new Error(`mail provider ${res.status}: ${detail}`);
  }
}

// ── Templates ───────────────────────────────────────────────────────────────
//
// Plain text first, with a small HTML twin. No tracking pixel, no shortened
// link, no remote image: a receipt should not phone home, and a download link
// that goes through a redirector is a link a spam filter distrusts.

const L = {
  en: {
    thanks: "Thank you for your purchase",
    order: "Order",
    downloads: "Your downloads",
    expires: "Link valid until",
    uses: "Downloads allowed",
    invoice: "Invoice",
    lost: "Lost the link? Ask for a new one at",
    refunded: "Your refund has been issued",
    refundBody: "We have refunded your order. The credit note is attached below.",
    resend: "Here is your download link again",
    total: "Total",
  },
  pl: {
    thanks: "Dziękujemy za zakup",
    order: "Zamówienie",
    downloads: "Twoje pliki",
    expires: "Link ważny do",
    uses: "Dozwolone pobrania",
    invoice: "Faktura",
    lost: "Zgubiłeś link? Poproś o nowy na",
    refunded: "Zwrot został zrealizowany",
    refundBody: "Zwróciliśmy płatność za zamówienie. Fakturę korygującą znajdziesz poniżej.",
    resend: "Oto ponownie Twój link do pobrania",
    total: "Razem",
  },
} as const;

export interface DownloadLink {
  name: string;
  url: string;
  expiresAt: string;
  maxUses: number;
}

export interface OrderMailInput {
  locale: string;
  shopName: string;
  orderNumber: string;
  totalMinor: number;
  currency: string;
  downloads: DownloadLink[];
  invoiceUrl?: string;
  resendUrl?: string;
  replyTo?: string;
}

export function orderPaidMail(input: OrderMailInput): MailPayload {
  const t = input.locale.startsWith("pl") ? L.pl : L.en;
  const total = formatMoney(input.totalMinor, input.currency, input.locale);

  const lines = [
    `${t.thanks}.`,
    "",
    `${t.order}: ${input.orderNumber}`,
    `${t.total}: ${total}`,
    "",
    `${t.downloads}:`,
    ...input.downloads.map(
      (d) => `- ${d.name}\n  ${d.url}\n  ${t.expires}: ${d.expiresAt.slice(0, 10)} · ${t.uses}: ${d.maxUses}`,
    ),
  ];
  if (input.invoiceUrl) lines.push("", `${t.invoice}: ${input.invoiceUrl}`);
  if (input.resendUrl) lines.push("", `${t.lost} ${input.resendUrl}`);

  const html = `<div style="font:14px/1.6 system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;color:#1f2328;max-width:34rem">
  <h1 style="font-size:1.2rem;color:#1967D2;margin:0 0 .5rem">${escapeHtml(t.thanks)}</h1>
  <p>${escapeHtml(t.order)}: <strong>${escapeHtml(input.orderNumber)}</strong><br>${escapeHtml(t.total)}: <strong>${escapeHtml(total)}</strong></p>
  <h2 style="font-size:1rem;margin:1.25rem 0 .5rem">${escapeHtml(t.downloads)}</h2>
  <ul style="padding-left:1.1rem">
    ${input.downloads
      .map(
        (d) =>
          `<li style="margin-bottom:.6rem"><a href="${escapeHtml(d.url)}" style="color:#1967D2">${escapeHtml(d.name)}</a><br>
           <small style="color:#5f6368">${escapeHtml(t.expires)}: ${escapeHtml(d.expiresAt.slice(0, 10))} · ${escapeHtml(t.uses)}: ${d.maxUses}</small></li>`,
      )
      .join("")}
  </ul>
  ${input.invoiceUrl ? `<p><a href="${escapeHtml(input.invoiceUrl)}" style="color:#1967D2">${escapeHtml(t.invoice)}</a></p>` : ""}
  ${input.resendUrl ? `<p style="color:#5f6368;font-size:.9rem">${escapeHtml(t.lost)} <a href="${escapeHtml(input.resendUrl)}" style="color:#1967D2">${escapeHtml(input.resendUrl)}</a></p>` : ""}
</div>`;

  const payload: MailPayload = {
    subject: `${t.thanks} — ${input.shopName} (${input.orderNumber})`,
    text: lines.join("\n"),
    html,
  };
  if (input.replyTo) payload.replyTo = input.replyTo;
  return payload;
}

export function resendMail(input: OrderMailInput): MailPayload {
  const t = input.locale.startsWith("pl") ? L.pl : L.en;
  const mail = orderPaidMail(input);
  return { ...mail, subject: `${t.resend} — ${input.shopName} (${input.orderNumber})` };
}

export function refundedMail(input: OrderMailInput & { creditNoteUrl?: string }): MailPayload {
  const t = input.locale.startsWith("pl") ? L.pl : L.en;
  const total = formatMoney(Math.abs(input.totalMinor), input.currency, input.locale);
  const text = [
    `${t.refunded}.`,
    "",
    t.refundBody,
    "",
    `${t.order}: ${input.orderNumber}`,
    `${t.total}: ${total}`,
    ...(input.creditNoteUrl ? ["", `${t.invoice}: ${input.creditNoteUrl}`] : []),
  ].join("\n");
  return { subject: `${t.refunded} — ${input.shopName} (${input.orderNumber})`, text };
}

/** What the shop owner gets. Deliberately thin: an order number, an amount and
 *  a country. Names and emails of buyers do not need to travel to a second
 *  inbox to tell the owner a sale happened (S-29). */
export function adminNoticeMail(kind: "paid" | "needs_review", data: {
  orderNumber: string;
  totalMinor: number;
  currency: string;
  country: string | null;
  gateway: string | null;
  reason?: string;
  panelUrl?: string;
}): MailPayload {
  const total = formatMoney(data.totalMinor, data.currency, "en");
  const subject =
    kind === "paid"
      ? `New order ${data.orderNumber} — ${total}`
      : `Order ${data.orderNumber} needs review — ${total}`;
  const text = [
    kind === "paid" ? "A new order has been paid." : "An order needs a decision before it can be fulfilled.",
    "",
    `Order:    ${data.orderNumber}`,
    `Amount:   ${total}`,
    `Country:  ${data.country ?? "—"}`,
    `Gateway:  ${data.gateway ?? "—"}`,
    ...(data.reason ? [`Reason:   ${data.reason}`] : []),
    ...(data.panelUrl ? ["", data.panelUrl] : []),
  ].join("\n");
  return { subject, text };
}
