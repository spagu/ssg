// The newsletter module's summary (GO-148): subscribers by status and the
// configuration problems that make sign-ups fail. A leading underscore keeps
// this file out of the Pages route table — module/summary.ts serves it.
//
// This template has no panel, so it lists nothing to do (an attention item
// links into a panel). newsletter-ui reads the same table: its summary shows
// the same numbers under the same keys and labels for confirmed and pending —
// workers/modules.test.ts runs both on one database — and adds what only it
// knows; a home page that sees both modules shows newsletter-ui's.

import { mailConfigProblem, turnstileSecret } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { statusCounts } from "./_store";
import { summaryOf } from "./_summary-kit";
import type { Metric, Problem, Summary, Text } from "./_summary-kit";

// The labels are newsletter-ui's dashboard tiles (tile.confirmed, …), word for word.
const LABELS: Record<"confirmed" | "pending" | "unsubscribed", Text> = {
  confirmed: { en: "Confirmed", pl: "Potwierdzeni", hi: "पुष्ट", de: "Bestätigt", fr: "Confirmés" },
  pending: { en: "Pending", pl: "Oczekujący", hi: "लंबित", de: "Unbestätigt", fr: "En attente" },
  unsubscribed: { en: "Unsubscribed", pl: "Wypisani", hi: "अनसब्सक्राइब", de: "Abgemeldet", fr: "Désabonnés" },
};

const PROBLEMS: Record<string, Text> = {
  no_database: {
    en: "NEWSLETTER_DB is not bound: every sign-up is refused.",
    pl: "NEWSLETTER_DB nie jest podpięta: każdy zapis jest odrzucany.",
    hi: "NEWSLETTER_DB बाउंड नहीं है: हर साइन-अप अस्वीकार किया जाता है।",
    de: "NEWSLETTER_DB ist nicht gebunden: Jede Anmeldung wird abgelehnt.",
    fr: "NEWSLETTER_DB n'est pas lié : chaque inscription est refusée.",
  },
  no_turnstile_secret: {
    en: "Neither NEWSLETTER_TURNSTILE_SECRET nor TURNSTILE_SECRET is set: every sign-up is refused.",
    pl: "Nie ustawiono ani NEWSLETTER_TURNSTILE_SECRET, ani TURNSTILE_SECRET: każdy zapis jest odrzucany.",
    hi: "न NEWSLETTER_TURNSTILE_SECRET सेट है, न TURNSTILE_SECRET: हर साइन-अप अस्वीकार किया जाता है।",
    de: "Weder NEWSLETTER_TURNSTILE_SECRET noch TURNSTILE_SECRET ist gesetzt: Jede Anmeldung wird abgelehnt.",
    fr: "Ni NEWSLETTER_TURNSTILE_SECRET ni TURNSTILE_SECRET n'est défini : chaque inscription est refusée.",
  },
  mail_unusable: {
    en: "Double opt-in is configured, but the mail transport can never send ({detail}): every sign-up is refused.",
    pl: "Podwójne potwierdzenie jest skonfigurowane, ale transport poczty nigdy nie wyśle wiadomości ({detail}): każdy zapis jest odrzucany.",
    hi: "डबल ऑप्ट-इन कॉन्फ़िगर है, लेकिन मेल ट्रांसपोर्ट कभी नहीं भेज सकता ({detail}): हर साइन-अप अस्वीकार किया जाता है।",
    de: "Double-Opt-in ist konfiguriert, aber der Mailversand kann nie senden ({detail}): Jede Anmeldung wird abgelehnt.",
    fr: "Le double opt-in est configuré, mais le transport d'e-mail ne peut jamais envoyer ({detail}) : chaque inscription est refusée.",
  },
  access_incomplete: {
    en: "Only one of NEWSLETTER_ACCESS_TEAM and NEWSLETTER_ACCESS_AUD is set, so Cloudflare Access is not used: the admin endpoint falls back to the password.",
    pl: "Ustawiono tylko jedną z NEWSLETTER_ACCESS_TEAM i NEWSLETTER_ACCESS_AUD, więc Cloudflare Access nie działa: punkt administracyjny wraca do hasła.",
    hi: "NEWSLETTER_ACCESS_TEAM और NEWSLETTER_ACCESS_AUD में से केवल एक सेट है, इसलिए Cloudflare Access का उपयोग नहीं होता: एडमिन एंडपॉइंट पासवर्ड पर लौट आता है।",
    de: "Nur eine von NEWSLETTER_ACCESS_TEAM und NEWSLETTER_ACCESS_AUD ist gesetzt, daher wird Cloudflare Access nicht verwendet: Der Admin-Endpunkt fällt auf das Passwort zurück.",
    fr: "Une seule des variables NEWSLETTER_ACCESS_TEAM et NEWSLETTER_ACCESS_AUD est définie, Cloudflare Access n'est donc pas utilisé : le point d'administration repasse au mot de passe.",
  },
};

// configProblems: what is wrong with the settings, named, never a secret's
// value. The mail problem carries the transport check's own (English)
// diagnosis — a port or a missing password, never the password.
export function configProblems(env: Env): Problem[] {
  const out: Problem[] = [];
  const add = (code: string, detail = "") => {
    const text = Object.fromEntries(Object.entries(PROBLEMS[code]).map(([l, s]) => [l, s.replace("{detail}", detail)])) as Text;
    out.push({ code, text });
  };
  if (!env.NEWSLETTER_DB) add("no_database");
  if (!turnstileSecret(env)) add("no_turnstile_secret");
  const mail = mailConfigProblem(env);
  if (mail) add("mail_unusable", mail);
  if (Boolean(env.NEWSLETTER_ACCESS_TEAM) !== Boolean(env.NEWSLETTER_ACCESS_AUD)) add("access_incomplete");
  return out;
}

// newsletterSummary is the whole answer of GET /api/newsletter/module/summary.
export async function newsletterSummary(env: Env, now: Date = new Date()): Promise<Summary> {
  const problems = configProblems(env);
  if (!env.NEWSLETTER_DB) return summaryOf({ metrics: [], problems }, now);
  await ensureSchema(env);
  const { byStatus } = await statusCounts(env);
  const metrics: Metric[] = (["confirmed", "pending", "unsubscribed"] as const).map((key) => ({ key, label: LABELS[key], value: byStatus[key] }));
  return summaryOf({ metrics, problems }, now);
}
