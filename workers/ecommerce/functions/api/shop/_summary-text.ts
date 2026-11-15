// The words of the shop's module summary (GO-156), in the five panel
// languages; _summary.ts decides which apply. A leading underscore keeps this
// file out of the Pages route table. Terms follow the panel and the other
// modules' summaries: "order", "refund", "VAT rate", "outgoing message".

import type { CountedText, Text } from "./_summary-kit";

export type MetricKey = "paidOrders" | "revenue" | "refunds" | "productsOnSale";

// LABELS: the metric tiles. `revenue` names its currency, which the summary
// format has no unit for — see revenueLabel.
export const LABELS: Record<Exclude<MetricKey, "revenue">, Text> = {
  paidOrders: { en: "Paid orders, last 30 days", pl: "Opłacone zamówienia, ostatnie 30 dni", hi: "भुगतान किए गए ऑर्डर, पिछले 30 दिन", de: "Bezahlte Bestellungen, letzte 30 Tage", fr: "Commandes payées, 30 derniers jours" },
  refunds: { en: "Refunds, last 30 days", pl: "Zwroty, ostatnie 30 dni", hi: "रिफ़ंड, पिछले 30 दिन", de: "Erstattungen, letzte 30 Tage", fr: "Remboursements, 30 derniers jours" },
  productsOnSale: { en: "Products on sale", pl: "Produkty w sprzedaży", hi: "बिक्री पर उत्पाद", de: "Produkte im Verkauf", fr: "Produits en vente" },
};

// revenueLabel: the revenue tile's label with the base currency in it — the
// value is in that currency's main unit (euros, not cents).
export function revenueLabel(currency: string): Text {
  return {
    en: `Revenue after refunds, last 30 days (${currency})`,
    pl: `Przychód po zwrotach, ostatnie 30 dni (${currency})`,
    hi: `रिफ़ंड के बाद आय, पिछले 30 दिन (${currency})`,
    de: `Umsatz nach Erstattungen, letzte 30 Tage (${currency})`,
    fr: `Chiffre d'affaires après remboursements, 30 derniers jours (${currency})`,
  };
}

export const REVIEW: CountedText = {
  en: { one: "{count} order needs your review before the buyer gets the files.", other: "{count} orders need your review before the buyers get their files." },
  pl: { one: "{count} zamówienie czeka na Twoje sprawdzenie, zanim kupujący dostanie pliki.", few: "{count} zamówienia czekają na Twoje sprawdzenie, zanim kupujący dostaną pliki.", many: "{count} zamówień czeka na Twoje sprawdzenie, zanim kupujący dostaną pliki.", other: "{count} zamówienia czeka na Twoje sprawdzenie, zanim kupujący dostaną pliki." },
  hi: { one: "{count} ऑर्डर आपकी जाँच का इंतज़ार कर रहा है, उसके बाद ही ख़रीदार को फ़ाइलें मिलेंगी।", other: "{count} ऑर्डर आपकी जाँच का इंतज़ार कर रहे हैं, उसके बाद ही ख़रीदारों को फ़ाइलें मिलेंगी।" },
  de: { one: "{count} Bestellung wartet auf Ihre Prüfung, bevor der Käufer die Dateien erhält.", other: "{count} Bestellungen warten auf Ihre Prüfung, bevor die Käufer ihre Dateien erhalten." },
  fr: { one: "{count} commande attend votre vérification avant que l'acheteur reçoive ses fichiers.", other: "{count} commandes attendent votre vérification avant que les acheteurs reçoivent leurs fichiers." },
};

export const UNDELIVERED: CountedText = {
  en: { one: "{count} paid order has no download link yet: the buyer is waiting for the file.", other: "{count} paid orders have no download link yet: the buyers are waiting for their files." },
  pl: { one: "{count} opłacone zamówienie nie ma jeszcze linku do pobrania: kupujący czeka na plik.", few: "{count} opłacone zamówienia nie mają jeszcze linku do pobrania: kupujący czekają na pliki.", many: "{count} opłaconych zamówień nie ma jeszcze linku do pobrania: kupujący czekają na pliki.", other: "{count} opłaconego zamówienia nie ma jeszcze linku do pobrania: kupujący czekają na pliki." },
  hi: { one: "{count} भुगतान किए गए ऑर्डर का डाउनलोड लिंक अभी नहीं बना है: ख़रीदार फ़ाइल का इंतज़ार कर रहा है।", other: "{count} भुगतान किए गए ऑर्डरों के डाउनलोड लिंक अभी नहीं बने हैं: ख़रीदार फ़ाइलों का इंतज़ार कर रहे हैं।" },
  de: { one: "{count} bezahlte Bestellung hat noch keinen Download-Link: Der Käufer wartet auf die Datei.", other: "{count} bezahlte Bestellungen haben noch keinen Download-Link: Die Käufer warten auf ihre Dateien." },
  fr: { one: "{count} commande payée n'a pas encore de lien de téléchargement : l'acheteur attend son fichier.", other: "{count} commandes payées n'ont pas encore de lien de téléchargement : les acheteurs attendent leurs fichiers." },
};

export const OUTBOX_FAILED: CountedText = {
  en: { one: "{count} outgoing message (an email or a webhook) keeps failing and needs attention.", other: "{count} outgoing messages (emails or webhooks) keep failing and need attention." },
  pl: { one: "{count} wychodząca wiadomość (e-mail albo webhook) wciąż się nie wysyła i wymaga uwagi.", few: "{count} wychodzące wiadomości (e-maile albo webhooki) wciąż się nie wysyłają i wymagają uwagi.", many: "{count} wychodzących wiadomości (e-maili albo webhooków) wciąż się nie wysyła i wymaga uwagi.", other: "{count} wychodzącej wiadomości (e-maili albo webhooków) wciąż się nie wysyła i wymaga uwagi." },
  hi: { one: "{count} बाहर जाने वाला संदेश (ईमेल या वेबहुक) बार-बार विफल हो रहा है और ध्यान माँगता है।", other: "{count} बाहर जाने वाले संदेश (ईमेल या वेबहुक) बार-बार विफल हो रहे हैं और ध्यान माँगते हैं।" },
  de: { one: "{count} ausgehende Nachricht (E-Mail oder Webhook) schlägt wiederholt fehl und braucht Aufmerksamkeit.", other: "{count} ausgehende Nachrichten (E-Mails oder Webhooks) schlagen wiederholt fehl und brauchen Aufmerksamkeit." },
  fr: { one: "{count} message sortant (e-mail ou webhook) échoue sans cesse et demande votre attention.", other: "{count} messages sortants (e-mails ou webhooks) échouent sans cesse et demandent votre attention." },
};

export const WEBHOOK_ERRORS: CountedText = {
  en: { one: "{count} payment notification from Stripe or PayPal could not be applied in the last 7 days.", other: "{count} payment notifications from Stripe or PayPal could not be applied in the last 7 days." },
  pl: { one: "{count} powiadomienie o płatności od Stripe albo PayPal nie zostało zastosowane w ostatnich 7 dniach.", few: "{count} powiadomienia o płatności od Stripe albo PayPal nie zostały zastosowane w ostatnich 7 dniach.", many: "{count} powiadomień o płatności od Stripe albo PayPal nie zostało zastosowanych w ostatnich 7 dniach.", other: "{count} powiadomienia o płatności od Stripe albo PayPal nie zostało zastosowane w ostatnich 7 dniach." },
  hi: { one: "पिछले 7 दिनों में Stripe या PayPal की {count} भुगतान सूचना लागू नहीं हो सकी।", other: "पिछले 7 दिनों में Stripe या PayPal की {count} भुगतान सूचनाएँ लागू नहीं हो सकीं।" },
  de: { one: "{count} Zahlungsbenachrichtigung von Stripe oder PayPal konnte in den letzten 7 Tagen nicht verarbeitet werden.", other: "{count} Zahlungsbenachrichtigungen von Stripe oder PayPal konnten in den letzten 7 Tagen nicht verarbeitet werden." },
  fr: { one: "{count} notification de paiement de Stripe ou PayPal n'a pas pu être appliquée ces 7 derniers jours.", other: "{count} notifications de paiement de Stripe ou PayPal n'ont pas pu être appliquées ces 7 derniers jours." },
};

export const VAT_ENDING: CountedText = {
  en: { one: "{count} VAT rate ends within 30 days and has no rate to follow it.", other: "{count} VAT rates end within 30 days and have no rates to follow them." },
  pl: { one: "{count} stawka VAT wygasa w ciągu 30 dni i nie ma następczyni.", few: "{count} stawki VAT wygasają w ciągu 30 dni i nie mają następczyń.", many: "{count} stawek VAT wygasa w ciągu 30 dni i nie ma następczyń.", other: "{count} stawki VAT wygasa w ciągu 30 dni i nie ma następczyń." },
  hi: { one: "{count} VAT दर 30 दिनों में ख़त्म हो रही है और उसके बाद कोई दर तय नहीं है।", other: "{count} VAT दरें 30 दिनों में ख़त्म हो रही हैं और उनके बाद कोई दर तय नहीं है।" },
  de: { one: "{count} Mehrwertsteuersatz endet in den nächsten 30 Tagen, ohne dass ein Nachfolger eingetragen ist.", other: "{count} Mehrwertsteuersätze enden in den nächsten 30 Tagen, ohne dass Nachfolger eingetragen sind." },
  fr: { one: "{count} taux de TVA prend fin dans les 30 jours sans taux pour lui succéder.", other: "{count} taux de TVA prennent fin dans les 30 jours sans taux pour leur succéder." },
};

export const NOTES: Record<"stripeTest" | "paypalSandbox" | "vatStale", Text> = {
  stripeTest: {
    en: "Stripe uses a test key (sk_test_…) on a public site: buyers cannot really pay.",
    pl: "Stripe używa klucza testowego (sk_test_…) na publicznej stronie: kupujący nie mogą naprawdę zapłacić.",
    hi: "Stripe सार्वजनिक साइट पर टेस्ट कुंजी (sk_test_…) इस्तेमाल कर रहा है: ख़रीदार असल में भुगतान नहीं कर सकते।",
    de: "Stripe verwendet auf einer öffentlichen Website einen Testschlüssel (sk_test_…): Käufer können nicht wirklich bezahlen.",
    fr: "Stripe utilise une clé de test (sk_test_…) sur un site public : les acheteurs ne peuvent pas vraiment payer.",
  },
  paypalSandbox: {
    en: "PayPal runs in its sandbox on a public site (SHOP_PAYPAL_ENV is not \"live\"): buyers cannot really pay.",
    pl: "PayPal działa w trybie sandbox na publicznej stronie (SHOP_PAYPAL_ENV nie jest „live”): kupujący nie mogą naprawdę zapłacić.",
    hi: "PayPal सार्वजनिक साइट पर सैंडबॉक्स में चल रहा है (SHOP_PAYPAL_ENV \"live\" नहीं है): ख़रीदार असल में भुगतान नहीं कर सकते।",
    de: "PayPal läuft auf einer öffentlichen Website in der Sandbox (SHOP_PAYPAL_ENV ist nicht „live“): Käufer können nicht wirklich bezahlen.",
    fr: "PayPal fonctionne en mode sandbox sur un site public (SHOP_PAYPAL_ENV n'est pas « live ») : les acheteurs ne peuvent pas vraiment payer.",
  },
  vatStale: {
    en: "The VAT rates were last changed over a year ago. Check them against the current rates.",
    pl: "Stawki VAT zmieniono ostatnio ponad rok temu. Porównaj je z aktualnymi.",
    hi: "VAT दरें आख़िरी बार एक साल से पहले बदली गई थीं। उन्हें मौजूदा दरों से मिला लें।",
    de: "Die Mehrwertsteuersätze wurden zuletzt vor über einem Jahr geändert. Gleichen Sie sie mit den aktuellen Sätzen ab.",
    fr: "Les taux de TVA ont été modifiés pour la dernière fois il y a plus d'un an. Comparez-les aux taux en vigueur.",
  },
};

export type ProblemCode = "no_database" | "no_files" | "no_kv" | "no_gateway" | "no_mail" | "no_seller_details";

export const PROBLEMS: Record<ProblemCode, Text> = {
  no_database: {
    en: "SHOP_DB (D1) is not bound: the shop can store no order and sell nothing.",
    pl: "SHOP_DB (D1) nie jest podpięta: sklep nie zapisze żadnego zamówienia i nic nie sprzeda.",
    hi: "SHOP_DB (D1) बाउंड नहीं है: दुकान कोई ऑर्डर सहेज नहीं सकती और कुछ नहीं बेच सकती।",
    de: "SHOP_DB (D1) ist nicht gebunden: Der Shop kann keine Bestellung speichern und nichts verkaufen.",
    fr: "SHOP_DB (D1) n'est pas lié : la boutique ne peut enregistrer aucune commande ni rien vendre.",
  },
  no_files: {
    en: "SHOP_FILES (R2) is not bound: bought files cannot be delivered.",
    pl: "SHOP_FILES (R2) nie jest podpięty: kupionych plików nie da się dostarczyć.",
    hi: "SHOP_FILES (R2) बाउंड नहीं है: ख़रीदी गई फ़ाइलें पहुँचाई नहीं जा सकतीं।",
    de: "SHOP_FILES (R2) ist nicht gebunden: Gekaufte Dateien können nicht ausgeliefert werden.",
    fr: "SHOP_FILES (R2) n'est pas lié : les fichiers achetés ne peuvent pas être livrés.",
  },
  no_kv: {
    en: "SHOP_KV (KV) is not bound: download windows are off, and without a rate-limiting binding no request budget is enforced.",
    pl: "SHOP_KV (KV) nie jest podpięty: okna pobierania są wyłączone, a bez wiązania rate limiting żaden limit zapytań nie działa.",
    hi: "SHOP_KV (KV) बाउंड नहीं है: डाउनलोड विंडो बंद हैं, और रेट-लिमिटिंग बाइंडिंग के बिना कोई अनुरोध सीमा लागू नहीं होती।",
    de: "SHOP_KV (KV) ist nicht gebunden: Download-Fenster sind aus, und ohne Rate-Limiting-Binding gilt kein Anfragebudget.",
    fr: "SHOP_KV (KV) n'est pas lié : les fenêtres de téléchargement sont désactivées et, sans liaison de limitation de débit, aucun quota de requêtes ne s'applique.",
  },
  no_gateway: {
    en: "No payment provider is configured (Stripe needs STRIPE_SECRET_KEY; PayPal needs PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET): nothing can be bought.",
    pl: "Nie skonfigurowano żadnego operatora płatności (Stripe wymaga STRIPE_SECRET_KEY, PayPal — PAYPAL_CLIENT_ID i PAYPAL_CLIENT_SECRET): nic nie da się kupić.",
    hi: "कोई भुगतान प्रदाता सेट नहीं है (Stripe को STRIPE_SECRET_KEY चाहिए; PayPal को PAYPAL_CLIENT_ID और PAYPAL_CLIENT_SECRET): कुछ भी ख़रीदा नहीं जा सकता।",
    de: "Kein Zahlungsanbieter ist eingerichtet (Stripe braucht STRIPE_SECRET_KEY, PayPal PAYPAL_CLIENT_ID und PAYPAL_CLIENT_SECRET): Nichts kann gekauft werden.",
    fr: "Aucun prestataire de paiement n'est configuré (Stripe demande STRIPE_SECRET_KEY ; PayPal, PAYPAL_CLIENT_ID et PAYPAL_CLIENT_SECRET) : rien ne peut être acheté.",
  },
  no_mail: {
    en: "Email is not configured (SHOP_MAIL_URL and SHOP_MAIL_FROM): buyers get no download links by email.",
    pl: "Poczta nie jest skonfigurowana (SHOP_MAIL_URL i SHOP_MAIL_FROM): kupujący nie dostają linków do pobrania e-mailem.",
    hi: "ईमेल सेट नहीं है (SHOP_MAIL_URL और SHOP_MAIL_FROM): ख़रीदारों को ईमेल से डाउनलोड लिंक नहीं मिलते।",
    de: "E-Mail ist nicht eingerichtet (SHOP_MAIL_URL und SHOP_MAIL_FROM): Käufer erhalten keine Download-Links per E-Mail.",
    fr: "L'e-mail n'est pas configuré (SHOP_MAIL_URL et SHOP_MAIL_FROM) : les acheteurs ne reçoivent pas de lien de téléchargement par e-mail.",
  },
  no_seller_details: {
    en: "The seller's name, address or country is missing in Settings: invoices are issued without them.",
    pl: "W ustawieniach brakuje nazwy, adresu albo kraju sprzedawcy: faktury wystawiane są bez nich.",
    hi: "सेटिंग्स में विक्रेता का नाम, पता या देश नहीं है: चालान उनके बिना बनते हैं।",
    de: "In den Einstellungen fehlen Name, Anschrift oder Land des Verkäufers: Rechnungen werden ohne sie ausgestellt.",
    fr: "Le nom, l'adresse ou le pays du vendeur manque dans les paramètres : les factures sont émises sans eux.",
  },
};
