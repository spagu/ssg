// The comments module's summary (GO-148): counts by status, the moderation
// queue as something to do, configuration problems. A leading underscore keeps
// this file out of the Pages route table — module/summary.ts serves it.
//
// One aggregate statement over the (status, created_at) index, whatever the
// table's size; never a comment's author, address or text.

import { turnstileSecret } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { counted, summaryOf } from "./_summary-kit";
import type { Attention, CountedText, Metric, Problem, Summary, Text } from "./_summary-kit";

// PANEL is where every attention item links: the moderation panel.
export const PANEL = "/comments-admin.html";
// STALE_DAYS: a queue whose oldest comment has waited this long is a warning.
export const STALE_DAYS = 7;

const LABELS: Record<"pending" | "approved" | "spam", Text> = {
  pending: { en: "Awaiting moderation", pl: "Czekają na moderację", hi: "मॉडरेशन की प्रतीक्षा में", de: "Warten auf Moderation", fr: "En attente de modération" },
  approved: { en: "Approved", pl: "Zatwierdzone", hi: "स्वीकृत", de: "Freigegeben", fr: "Approuvés" },
  spam: { en: "Held as spam", pl: "Zatrzymane jako spam", hi: "स्पैम के रूप में रोकी गईं", de: "Als Spam zurückgehalten", fr: "Retenus comme spam" },
};

const AWAITING: CountedText = {
  en: { one: "{count} comment awaits moderation.", other: "{count} comments await moderation." },
  pl: { one: "{count} komentarz czeka na moderację.", few: "{count} komentarze czekają na moderację.", many: "{count} komentarzy czeka na moderację.", other: "{count} komentarza czeka na moderację." },
  hi: { one: "{count} टिप्पणी मॉडरेशन की प्रतीक्षा में है।", other: "{count} टिप्पणियाँ मॉडरेशन की प्रतीक्षा में हैं।" },
  de: { one: "{count} Kommentar wartet auf Moderation.", other: "{count} Kommentare warten auf Moderation." },
  fr: { one: "{count} commentaire attend la modération.", other: "{count} commentaires attendent la modération." },
};

const STALE: CountedText = {
  en: { one: "The oldest comment in the queue has been waiting for {count} day.", other: "The oldest comment in the queue has been waiting for {count} days." },
  pl: { one: "Najstarszy komentarz w kolejce czeka od {count} dnia.", other: "Najstarszy komentarz w kolejce czeka od {count} dni." },
  hi: { one: "कतार की सबसे पुरानी टिप्पणी {count} दिन से प्रतीक्षा में है।", other: "कतार की सबसे पुरानी टिप्पणी {count} दिनों से प्रतीक्षा में है।" },
  de: { one: "Der älteste Kommentar in der Warteschlange wartet seit {count} Tag.", other: "Der älteste Kommentar in der Warteschlange wartet seit {count} Tagen." },
  fr: { one: "Le plus ancien commentaire de la file attend depuis {count} jour.", other: "Le plus ancien commentaire de la file attend depuis {count} jours." },
};

const PROBLEMS: Record<string, Text> = {
  no_database: {
    en: "COMMENTS_DB is not bound: comments can be neither stored nor moderated.",
    pl: "COMMENTS_DB nie jest podpięta: komentarzy nie da się ani zapisać, ani moderować.",
    hi: "COMMENTS_DB बाउंड नहीं है: टिप्पणियाँ न सहेजी जा सकती हैं, न मॉडरेट की जा सकती हैं।",
    de: "COMMENTS_DB ist nicht gebunden: Kommentare können weder gespeichert noch moderiert werden.",
    fr: "COMMENTS_DB n'est pas lié : les commentaires ne peuvent être ni enregistrés ni modérés.",
  },
  no_turnstile_secret: {
    en: "Neither COMMENTS_TURNSTILE_SECRET nor TURNSTILE_SECRET is set: every new comment is refused.",
    pl: "Nie ustawiono ani COMMENTS_TURNSTILE_SECRET, ani TURNSTILE_SECRET: każdy nowy komentarz jest odrzucany.",
    hi: "न COMMENTS_TURNSTILE_SECRET सेट है, न TURNSTILE_SECRET: हर नई टिप्पणी अस्वीकार की जाती है।",
    de: "Weder COMMENTS_TURNSTILE_SECRET noch TURNSTILE_SECRET ist gesetzt: Jeder neue Kommentar wird abgelehnt.",
    fr: "Ni COMMENTS_TURNSTILE_SECRET ni TURNSTILE_SECRET n'est défini : chaque nouveau commentaire est refusé.",
  },
  access_incomplete: {
    en: "Only one of COMMENTS_ACCESS_TEAM and COMMENTS_ACCESS_AUD is set, so Cloudflare Access is not used: moderation falls back to the password.",
    pl: "Ustawiono tylko jedną z COMMENTS_ACCESS_TEAM i COMMENTS_ACCESS_AUD, więc Cloudflare Access nie działa: moderacja wraca do hasła.",
    hi: "COMMENTS_ACCESS_TEAM और COMMENTS_ACCESS_AUD में से केवल एक सेट है, इसलिए Cloudflare Access का उपयोग नहीं होता: मॉडरेशन पासवर्ड पर लौट आता है।",
    de: "Nur eine von COMMENTS_ACCESS_TEAM und COMMENTS_ACCESS_AUD ist gesetzt, daher wird Cloudflare Access nicht verwendet: Die Moderation fällt auf das Passwort zurück.",
    fr: "Une seule des variables COMMENTS_ACCESS_TEAM et COMMENTS_ACCESS_AUD est définie, Cloudflare Access n'est donc pas utilisé : la modération repasse au mot de passe.",
  },
};

// configProblems: what is wrong with the settings, named, never their values.
export function configProblems(env: Env): Problem[] {
  const codes: string[] = [];
  if (!env.COMMENTS_DB) codes.push("no_database");
  if (!turnstileSecret(env)) codes.push("no_turnstile_secret");
  if (!env.COMMENTS_ACCESS_TEAM !== !env.COMMENTS_ACCESS_AUD) codes.push("access_incomplete");
  return codes.map((code) => ({ code, text: PROBLEMS[code] }));
}

// commentsSummary is the whole answer of GET /api/comments/module/summary.
export async function commentsSummary(env: Env, now: Date = new Date()): Promise<Summary> {
  const problems = configProblems(env);
  if (!env.COMMENTS_DB) return summaryOf({ metrics: [], problems }, now);
  await ensureSchema(env);
  const { results } = await env.COMMENTS_DB.prepare(
    "SELECT status, COUNT(*) AS n, MIN(created_at) AS oldest FROM comments GROUP BY status",
  ).all<{ status: string; n: number; oldest: string }>();
  const row = (status: string) => results.find((r) => r.status === status);
  const count = (status: string) => Number(row(status)?.n ?? 0);

  const metrics: Metric[] = (["pending", "approved", "spam"] as const).map((key) => ({ key, label: LABELS[key], value: count(key) }));
  const attention: Attention[] = [];
  const pending = count("pending");
  if (pending > 0) {
    attention.push({ severity: "action", text: counted(AWAITING, pending), href: PANEL });
    const waited = Math.floor((now.getTime() - Date.parse(row("pending")!.oldest)) / 86_400_000);
    if (waited >= STALE_DAYS) attention.push({ severity: "warning", text: counted(STALE, waited), href: PANEL });
  }
  return summaryOf({ metrics, attention, problems }, now);
}
