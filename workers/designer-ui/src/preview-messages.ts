// What the preview frame says when there is nothing to show (GO-188), in the
// panel's five languages. The frame is sandboxed, so the panel cannot read
// its status; the page explains itself instead. Text only, escaped by
// errorPage: nothing of the request is echoed back into it.

export const LANGS = ["en", "pl", "hi", "de", "fr"] as const;
export type Lang = (typeof LANGS)[number];

type Texts = Record<string, string>;

const MESSAGES: Record<Lang, Texts> = {
  en: {
    title: "No preview",
    bad_path: "That is not a path on this site. Write it as /blog/my-post/.",
    bad_token: "A colour or font value could not be used in the preview.",
    redirect_off_host: "The page redirects to another host. Only this site's pages are previewed.",
    too_many_redirects: "The page redirects too many times.",
    page_not_found: "There is no page at that path.",
    page_unavailable: "The site did not answer with the page. Try again in a moment.",
    not_html: "That address is not a page (it is not HTML).",
    page_too_large: "The page is too large to preview on this plan: 256 KiB on Free, 2 MB on Paid (DESIGNER_PLAN).",
    upstream_timeout: "The site did not send the page within 10 seconds. Try again in a moment.",
    no_latest_post: "The latest post could not be found: the home page names no feed on this site, or the feed has no entries.",
  },
  pl: {
    title: "Brak podglądu",
    bad_path: "To nie jest ścieżka w tej witrynie. Wpisz ją jako /blog/moj-wpis/.",
    bad_token: "Wartości koloru lub czcionki nie da się użyć w podglądzie.",
    redirect_off_host: "Strona przekierowuje na inny host. Podgląd pokazuje tylko strony tej witryny.",
    too_many_redirects: "Strona przekierowuje zbyt wiele razy.",
    page_not_found: "Pod tą ścieżką nie ma strony.",
    page_unavailable: "Witryna nie odpowiedziała stroną. Spróbuj ponownie za chwilę.",
    not_html: "Ten adres nie jest stroną (to nie jest HTML).",
    page_too_large: "Strona jest za duża na podgląd w tym planie: 256 KiB w Free, 2 MB w Paid (DESIGNER_PLAN).",
    upstream_timeout: "Witryna nie przesłała strony w ciągu 10 sekund. Spróbuj ponownie za chwilę.",
    no_latest_post: "Nie znaleziono najnowszego wpisu: strona główna nie wskazuje kanału w tej witrynie albo kanał nie ma wpisów.",
  },
  hi: {
    title: "पूर्वावलोकन नहीं",
    bad_path: "यह इस साइट पर कोई पथ नहीं है। इसे /blog/my-post/ की तरह लिखें।",
    bad_token: "किसी रंग या फ़ॉन्ट का मान पूर्वावलोकन में इस्तेमाल नहीं हो सका।",
    redirect_off_host: "यह पेज किसी दूसरे होस्ट पर भेजता है। केवल इसी साइट के पेजों का पूर्वावलोकन होता है।",
    too_many_redirects: "यह पेज बहुत बार रीडायरेक्ट करता है।",
    page_not_found: "उस पथ पर कोई पेज नहीं है।",
    page_unavailable: "साइट ने पेज नहीं लौटाया। थोड़ी देर बाद फिर कोशिश करें।",
    not_html: "यह पता कोई पेज नहीं है (यह HTML नहीं है)।",
    page_too_large: "इस प्लान पर पूर्वावलोकन के लिए पेज बहुत बड़ा है: Free पर 256 KiB, Paid पर 2 MB (DESIGNER_PLAN)।",
    upstream_timeout: "साइट ने 10 सेकंड के भीतर पेज नहीं भेजा। थोड़ी देर बाद फिर कोशिश करें।",
    no_latest_post: "नवीनतम पोस्ट नहीं मिली: होम पेज इस साइट पर किसी फ़ीड का नाम नहीं देता, या फ़ीड में कोई प्रविष्टि नहीं है।",
  },
  de: {
    title: "Keine Vorschau",
    bad_path: "Das ist kein Pfad auf dieser Website. Schreiben Sie ihn als /blog/mein-beitrag/.",
    bad_token: "Ein Farb- oder Schriftwert ließ sich in der Vorschau nicht verwenden.",
    redirect_off_host: "Die Seite leitet auf einen anderen Host weiter. Die Vorschau zeigt nur Seiten dieser Website.",
    too_many_redirects: "Die Seite leitet zu oft weiter.",
    page_not_found: "Unter diesem Pfad gibt es keine Seite.",
    page_unavailable: "Die Website hat die Seite nicht geliefert. Versuchen Sie es gleich noch einmal.",
    not_html: "Diese Adresse ist keine Seite (kein HTML).",
    page_too_large: "Die Seite ist für die Vorschau in diesem Tarif zu groß: 256 KiB bei Free, 2 MB bei Paid (DESIGNER_PLAN).",
    upstream_timeout: "Die Website hat die Seite nicht innerhalb von 10 Sekunden geliefert. Versuchen Sie es gleich noch einmal.",
    no_latest_post: "Der neueste Beitrag wurde nicht gefunden: Die Startseite nennt keinen Feed auf dieser Website, oder der Feed hat keine Einträge.",
  },
  fr: {
    title: "Pas d’aperçu",
    bad_path: "Ce n’est pas un chemin de ce site. Écrivez-le sous la forme /blog/mon-article/.",
    bad_token: "Une valeur de couleur ou de police n’a pas pu être utilisée dans l’aperçu.",
    redirect_off_host: "La page redirige vers un autre hôte. L’aperçu ne montre que les pages de ce site.",
    too_many_redirects: "La page redirige trop de fois.",
    page_not_found: "Il n’y a pas de page à ce chemin.",
    page_unavailable: "Le site n’a pas renvoyé la page. Réessayez dans un instant.",
    not_html: "Cette adresse n’est pas une page (ce n’est pas du HTML).",
    page_too_large: "La page est trop grande pour l’aperçu avec ce forfait : 256 Kio en Free, 2 Mo en Paid (DESIGNER_PLAN).",
    upstream_timeout: "Le site n’a pas envoyé la page en moins de 10 secondes. Réessayez dans un instant.",
    no_latest_post: "L’article le plus récent est introuvable : la page d’accueil n’indique aucun flux sur ce site, ou le flux n’a aucune entrée.",
  },
};

// langOf: one of LANGS, English for anything else.
export const langOf = (value: string | null): Lang => (LANGS as readonly string[]).includes(value ?? "") ? (value as Lang) : "en";

const esc = (s: string): string => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

// errorPage: a small page of our own for the frame: the reason and its code.
export function errorPage(lang: Lang, code: string): string {
  const m = MESSAGES[lang];
  const text = m[code] ?? m.page_unavailable;
  return (
    `<!doctype html><html lang="${lang}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">` +
    `<meta name="robots" content="noindex, nofollow"><title>${esc(m.title)}</title><style>` +
    ":root{color-scheme:light dark}body{margin:0;padding:1.5rem;font:1rem/1.5 system-ui,sans-serif;color:#1f1f1f;background:#fff}" +
    "h1{font-size:1.25rem;margin:0 0 .5rem}code{font-size:.875rem;color:#424242}" +
    "@media (prefers-color-scheme:dark){body{color:#e8eaed;background:#202124}code{color:#bdc1c6}}" +
    `</style></head><body><h1>${esc(m.title)}</h1><p>${esc(text)}</p><p><code>${esc(code)}</code></p></body></html>`
  );
}
