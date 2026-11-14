// Invented modules for the dev server, the screenshots and the tests: a
// registry and, per scenario, what each fake module answers. Only example.com
// style data; the "hostile" module's texts carry markup and script that must
// come out as inert text.
import { MODULE } from "../functions/api/core/_module.ts";
import { entry, manifest, registry, text } from "./harness.ts";

const t5 = (en: string, pl: string, hi: string, de: string, fr: string) => ({ en, pl, hi, de, fr });
const HOSTILE = '<img src=x onerror="alert(1)"><script>alert(2)</script>';

export const MANIFESTS = {
  comments: manifest("comments", { nav: [["/comments-admin.html", "comments:read"]], endpoints: { manifest: "/api/comments/module", summary: "/api/comments/module/summary" }, version: "1.4.0" }),
  newsletter: manifest("newsletter", { perms: ["newsletter:read"], endpoints: { manifest: "/api/newsletter/module", summary: "/api/newsletter/module/summary" }, version: "1.3.2" }),
  "newsletter-ui": manifest("newsletter-ui", { kind: "worker", nav: [["/", "newsletter:read"]], endpoints: { manifest: "/api/module", health: "/api/health", summary: "/api/module/summary" }, version: "1.0.0" }),
  auth: manifest("auth", { perms: ["auth:admin"], endpoints: { manifest: "/api/auth/module" }, version: "1.0.0" }),
  "rate-limit": manifest("rate-limit", { endpoints: {}, version: "1.1.0" }),
  "kitchen-log": manifest("kitchen-log", { nav: [["/", "kitchen-log:read"]], endpoints: { manifest: "/api/module", summary: "/api/module/summary" }, version: "0.3.0" }),
};
// Titles people would see, instead of the test builder's "<id> title".
const TITLES: Record<string, ReturnType<typeof t5>> = {
  comments: t5("Comments", "Komentarze", "टिप्पणियाँ", "Kommentare", "Commentaires"),
  newsletter: t5("Newsletter sign-up", "Zapisy do newslettera", "न्यूज़लेटर साइन-अप", "Newsletter-Anmeldung", "Inscriptions à la newsletter"),
  "newsletter-ui": t5("Newsletter", "Newsletter", "न्यूज़लेटर", "Newsletter", "Newsletter"),
  auth: t5("Accounts", "Konta", "खाते", "Konten", "Comptes"),
  "rate-limit": t5("Rate limit", "Limit żądań", "अनुरोध सीमा", "Ratenbegrenzung", "Limite de requêtes"),
  "kitchen-log": t5(`Kitchen log ${HOSTILE}`, `Dziennik kuchni ${HOSTILE}`, `रसोई लॉग ${HOSTILE}`, `Küchenbuch ${HOSTILE}`, `Journal de cuisine ${HOSTILE}`),
};
for (const [id, m] of Object.entries(MANIFESTS)) (m as { title: unknown }).title = TITLES[id];

export const BASES: Record<string, string> = { "newsletter-ui": "/newsletter-admin", "kitchen-log": "/kitchen" };
export const REGISTRY = {
  ...registry(...Object.entries(MANIFESTS).map(([id, m]) => entry(id, m, BASES[id] ?? "/")), entry("core", MODULE)),
  // Templates the site does not use (GO-149's "available, not connected").
  available: [
    { id: "contact-form", kind: "pages-function", title: "Contact form" },
    { id: "conversions-proxy", kind: "pages-function", title: "Conversions proxy" },
    { id: "web-push", kind: "pages-function", title: "Web push" },
  ],
};

const metric = (key: string, label: ReturnType<typeof t5>, value: number, unit?: string) => ({ key, label, value, ...(unit ? { unit } : {}) });
const L = {
  pending: t5("Awaiting moderation", "Czekają na moderację", "मॉडरेशन की प्रतीक्षा में", "Warten auf Moderation", "En attente de modération"),
  approved: t5("Approved", "Zatwierdzone", "स्वीकृत", "Freigegeben", "Approuvés"),
  spam: t5("Spam", "Spam", "स्पैम", "Spam", "Spam"),
  confirmed: t5("Confirmed subscribers", "Potwierdzeni subskrybenci", "पुष्ट सब्सक्राइबर", "Bestätigte Abonnenten", "Abonnés confirmés"),
  waiting: t5("Awaiting confirmation", "Czekają na potwierdzenie", "पुष्टि की प्रतीक्षा में", "Warten auf Bestätigung", "En attente de confirmation"),
  unsub: t5("Unsubscribed", "Wypisani", "सदस्यता छोड़ी", "Abgemeldet", "Désinscrits"),
  sending: t5("Campaigns sending", "Kampanie w wysyłce", "भेजे जा रहे अभियान", "Laufende Kampagnen", "Campagnes en cours"),
  suppressed: t5("Suppressed addresses", "Zablokowane adresy", "रोके गए पते", "Gesperrte Adressen", "Adresses bloquées"),
  signups: t5("Sign-ups per day", "Zapisy dziennie", "प्रतिदिन साइन-अप", "Anmeldungen pro Tag", "Inscriptions par jour"),
  dishes: t5("Dishes logged", "Zapisane dania", "दर्ज व्यंजन", "Erfasste Gerichte", "Plats consignés"),
  share: t5("Logged on time", "Zapisane na czas", "समय पर दर्ज", "Pünktlich erfasst", "Consignés à temps"),
};

// days: the 30 UTC days up to 2026-11-13, a gentle weekly rhythm.
const series = () => ({
  label: L.signups,
  points: Array.from({ length: 30 }, (_, i) => ({ day: new Date(Date.UTC(2026, 9, 15 + i)).toISOString().slice(0, 10), value: 3 + ((i * 7) % 11) + (i % 7 === 5 ? 9 : 0) })),
});
const summary = (parts: Record<string, unknown>) => ({ contract: 1, generatedAt: "2026-11-13T09:30:00.000Z", metrics: [], attention: [], problems: [], ...parts });

export type Answer = { status: number; body?: unknown; delayMs?: number; raw?: string };
export type Scenario = Record<string, Answer>; // keyed by path on the host

const manifests = (): Scenario => ({
  "/api/comments/module": { status: 200, body: MANIFESTS.comments },
  "/api/newsletter/module": { status: 200, body: MANIFESTS.newsletter },
  "/newsletter-admin/api/health": { status: 200, raw: "ok" },
  "/api/auth/module": { status: 200, body: MANIFESTS.auth },
  "/kitchen/api/module": { status: 200, body: MANIFESTS["kitchen-log"] },
});
const BUILD = { generator: "ssg", version: "1.9.9", buildTime: "2026-11-13T07:12:00Z", started: "2026-11-13T07:12:00Z", finished: "2026-11-13T07:12:09Z", durationSeconds: 8.6, pages: 412, files: 1288, content: { post: 120, page: 14 } };

const good = (): Scenario => ({
  ...manifests(),
  "/build.json": { status: 200, body: BUILD },
  "/api/comments/module/summary": { status: 200, body: summary({ metrics: [metric("pending", L.pending, 0), metric("approved", L.approved, 1284), metric("spam", L.spam, 37)] }) },
  "/api/newsletter/module/summary": { status: 200, body: summary({ metrics: [metric("confirmed", L.confirmed, 1840), metric("pending", L.waiting, 23), metric("unsubscribed", L.unsub, 96)] }) },
  "/newsletter-admin/api/module/summary": {
    status: 200,
    body: summary({ metrics: [metric("confirmed", L.confirmed, 1840), metric("pending", L.waiting, 23), metric("campaignsInProgress", L.sending, 1), metric("suppressed", L.suppressed, 12)], series: series() }),
  },
  "/kitchen/api/module/summary": { status: 200, body: summary({ metrics: [metric("dishes", L.dishes, 318), metric("onTime", L.share, 92.5, "percent")] }) },
});

const todo = (): Scenario => {
  const s = good();
  s["/build.json"] = { status: 404, raw: "not found" };
  s["/api/comments/module/summary"] = {
    status: 200,
    body: summary({
      metrics: [metric("pending", L.pending, 12), metric("approved", L.approved, 1284), metric("spam", L.spam, 37)],
      attention: [
        { severity: "action", text: t5("12 comments await moderation.", "12 komentarzy czeka na moderację.", "12 टिप्पणियाँ मॉडरेशन की प्रतीक्षा में हैं।", "12 Kommentare warten auf Moderation.", "12 commentaires attendent une modération."), href: "/comments-admin.html" },
        { severity: "warning", text: t5("The oldest comment in the queue has been waiting for 9 days.", "Najstarszy komentarz w kolejce czeka od 9 dni.", "कतार की सबसे पुरानी टिप्पणी 9 दिनों से प्रतीक्षा में है।", "Der älteste Kommentar in der Warteschlange wartet seit 9 Tagen.", "Le plus ancien commentaire de la file attend depuis 9 jours."), href: "/comments-admin.html" },
      ],
    }),
  };
  s["/api/newsletter/module/summary"] = { status: 200, body: summary({ metrics: [metric("confirmed", L.confirmed, 1840)], problems: [{ code: "no_turnstile_secret", text: t5("TURNSTILE_SECRET is not set: every new sign-up is refused.", "TURNSTILE_SECRET nie jest ustawiony: każdy nowy zapis jest odrzucany.", "TURNSTILE_SECRET सेट नहीं है: हर नया साइन-अप अस्वीकार होता है।", "TURNSTILE_SECRET ist nicht gesetzt: Jede neue Anmeldung wird abgelehnt.", "TURNSTILE_SECRET n'est pas défini : chaque nouvelle inscription est refusée.") }] }) };
  const ui = (s["/newsletter-admin/api/module/summary"].body as { attention: unknown[] });
  ui.attention = [
    { severity: "action", text: t5("The send engine paused the campaign “Winter menu”.", "Silnik wysyłki wstrzymał kampanię „Zimowe menu”.", "भेजने वाले इंजन ने अभियान “विंटर मेन्यू” रोक दिया।", "Der Versand hat die Kampagne „Wintermenü“ angehalten.", "Le moteur d'envoi a mis en pause la campagne « Menu d'hiver »."), href: "/#/campaigns?status=paused" },
    { severity: "warning", text: t5("3 send jobs ran out of retries.", "3 zadania wysyłki wyczerpały ponowienia.", "3 भेजने के काम दोबारा कोशिशें ख़त्म कर चुके।", "3 Versandaufträge haben keine Wiederholungen mehr.", "3 tâches d'envoi ont épuisé leurs nouvelles tentatives."), href: "/#/" },
  ];
  s["/kitchen/api/module/summary"] = {
    status: 200,
    body: summary({
      metrics: [metric("dishes", L.dishes, 318)],
      attention: [{ severity: "info", text: text(`Menu photos are due on Friday. ${HOSTILE}`), href: "/#/photos" }],
      problems: [{ code: "no_photo_bucket", text: text(`PHOTOS is not bound: uploads are refused. ${HOSTILE}`) }],
    }),
  };
  return s;
};

const down = (): Scenario => {
  const s = good();
  s["/newsletter-admin/api/health"] = { status: 200, raw: "ok", delayMs: 60_000 };
  s["/newsletter-admin/api/module/summary"] = { status: 200, raw: "{}", delayMs: 60_000 };
  s["/kitchen/api/module/summary"] = { status: 200, raw: '{"contract": 1, "generatedAt": "yesterday", "metrics": "lots"}' };
  s["/api/comments/module/summary"] = { status: 500, raw: "boom" };
  s["/api/comments/module"] = { status: 500, raw: "boom" };
  return s;
};

export const SCENARIOS: Record<string, () => Scenario> = { good, todo, down };
export { HOSTILE };
