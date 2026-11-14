// The texts of the module summary (GO-148), in the panel's five languages.
// Data only: module-summary.ts decides when each is said.

import type { CountedText, Text } from "./summary-kit";

export const IN_PROGRESS: Text = {
  en: "Campaigns in progress",
  pl: "Kampanie w toku",
  hi: "चल रहे कैंपेन",
  de: "Laufende Kampagnen",
  fr: "Campagnes en cours",
};

export const PAUSED_BY_ENGINE: CountedText = {
  en: { one: "{count} campaign was paused by the send engine and waits for you.", other: "{count} campaigns were paused by the send engine and wait for you." },
  pl: {
    one: "Silnik wysyłki wstrzymał {count} kampanię; czeka na Ciebie.",
    few: "Silnik wysyłki wstrzymał {count} kampanie; czekają na Ciebie.",
    many: "Silnik wysyłki wstrzymał {count} kampanii; czekają na Ciebie.",
    other: "Silnik wysyłki wstrzymał {count} kampanii; czekają na Ciebie.",
  },
  hi: { one: "सेंड इंजन ने {count} कैंपेन रोक दिया है; वह आपकी प्रतीक्षा में है।", other: "सेंड इंजन ने {count} कैंपेन रोक दिए हैं; वे आपकी प्रतीक्षा में हैं।" },
  de: { one: "{count} Kampagne wurde vom Versandsystem pausiert und wartet auf Sie.", other: "{count} Kampagnen wurden vom Versandsystem pausiert und warten auf Sie." },
  fr: {
    one: "{count} campagne a été mise en pause par le moteur d'envoi et vous attend.",
    other: "{count} campagnes ont été mises en pause par le moteur d'envoi et vous attendent.",
  },
};

// The panel's warn.uncertain, without the campaign's subject: the summary
// counts across campaigns.
export const UNCERTAIN: CountedText = {
  en: { one: "{count} message may or may not have gone out. It is never re-sent without you.", other: "{count} messages may or may not have gone out. They are never re-sent without you." },
  pl: {
    one: "{count} wiadomość mogła wyjść albo nie. Bez Twojej decyzji nigdy nie zostanie wysłana ponownie.",
    few: "{count} wiadomości mogły wyjść albo nie. Bez Twojej decyzji nigdy nie zostaną wysłane ponownie.",
    many: "{count} wiadomości mogło wyjść albo nie. Bez Twojej decyzji nigdy nie zostaną wysłane ponownie.",
    other: "{count} wiadomości mogło wyjść albo nie. Bez Twojej decyzji nigdy nie zostaną wysłane ponownie.",
  },
  hi: { one: "{count} संदेश शायद गया हो, शायद नहीं। आपकी अनुमति के बिना इसे कभी दोबारा नहीं भेजा जाता।", other: "{count} संदेश शायद गए हों, शायद नहीं। आपकी अनुमति के बिना इन्हें कभी दोबारा नहीं भेजा जाता।" },
  de: { one: "Bei {count} Nachricht ist unklar, ob sie verschickt wurde. Sie wird nie ohne Sie erneut gesendet.", other: "Bei {count} Nachrichten ist unklar, ob sie verschickt wurden. Sie werden nie ohne Sie erneut gesendet." },
  fr: { one: "{count} message est peut-être parti, peut-être pas. Il n'est jamais renvoyé sans votre accord.", other: "{count} messages sont peut-être partis, peut-être pas. Ils ne sont jamais renvoyés sans votre accord." },
};

export const NEVER_MAILED: CountedText = {
  en: { one: "{count} pending subscriber has never been sent a confirmation email.", other: "{count} pending subscribers have never been sent a confirmation email." },
  pl: {
    one: "{count} oczekujący subskrybent nie dostał jeszcze maila z prośbą o potwierdzenie.",
    few: "{count} oczekujący subskrybenci nie dostali jeszcze maila z prośbą o potwierdzenie.",
    many: "{count} oczekujących subskrybentów nie dostało jeszcze maila z prośbą o potwierdzenie.",
    other: "{count} oczekującego subskrybenta nie dostało jeszcze maila z prośbą o potwierdzenie.",
  },
  hi: { one: "{count} लंबित सब्सक्राइबर को अब तक पुष्टि ईमेल नहीं भेजा गया है।", other: "{count} लंबित सब्सक्राइबरों को अब तक पुष्टि ईमेल नहीं भेजा गया है।" },
  de: { one: "{count} unbestätigter Abonnent hat noch nie eine Bestätigungsmail erhalten.", other: "{count} unbestätigte Abonnenten haben noch nie eine Bestätigungsmail erhalten." },
  fr: { one: "{count} abonné en attente n'a jamais reçu d'e-mail de confirmation.", other: "{count} abonnés en attente n'ont jamais reçu d'e-mail de confirmation." },
};

export const OWN_PROBLEMS = {
  no_subscribers_table: {
    en: "The subscribers table does not exist yet: deploy the newsletter template against the same D1 database and send it one request.",
    pl: "Tabela subskrybentów jeszcze nie istnieje: wdróż szablon newsletter na tej samej bazie D1 i wyślij do niego jedno żądanie.",
    hi: "सब्सक्राइबर टेबल अभी मौजूद नहीं है: उसी D1 डेटाबेस के साथ newsletter टेम्पलेट डिप्लॉय करें और उसे एक अनुरोध भेजें।",
    de: "Die Abonnententabelle existiert noch nicht: Veröffentlichen Sie die Vorlage newsletter mit derselben D1-Datenbank und senden Sie ihr eine Anfrage.",
    fr: "La table des abonnés n'existe pas encore : déployez le modèle newsletter sur la même base D1 et envoyez-lui une requête.",
  },
  transport_unusable: {
    en: "The configured mail transport can never send: {detail}",
    pl: "Skonfigurowany transport poczty nigdy nie wyśle wiadomości: {detail}",
    hi: "कॉन्फ़िगर किया गया मेल ट्रांसपोर्ट कभी नहीं भेज सकता: {detail}",
    de: "Der konfigurierte Mailversand kann nie senden: {detail}",
    fr: "Le transport d'e-mail configuré ne peut jamais envoyer : {detail}",
  },
} satisfies Record<string, Text>;

// Panel strings, word for word (test/module-summary.test.ts holds them to
// public/js/strings/*.js): a number or a problem reads the same on the home
// page as in the panel.
export const PANEL_LABELS = {
  confirmed: {
    en: "Confirmed",
    pl: "Potwierdzeni",
    hi: "पुष्ट",
    de: "Bestätigt",
    fr: "Confirmés"
  }, // tile.confirmed
  pending: {
    en: "Pending",
    pl: "Oczekujący",
    hi: "लंबित",
    de: "Unbestätigt",
    fr: "En attente"
  }, // tile.pending
  suppressed: {
    en: "Suppressed",
    pl: "Zablokowani",
    hi: "ब्लॉक",
    de: "Gesperrt",
    fr: "Exclus"
  }, // tile.suppressed
  signups: {
    en: "Sign-ups",
    pl: "Zapisy",
    hi: "साइन-अप",
    de: "Anmeldungen",
    fr: "Inscriptions"
  }, // series.signups
} satisfies Record<string, Text>;

export const DEAD_LETTERS: CountedText = {
  en: {
    one: "{count} send job ran out of retries. Its recipients are re-queued automatically after the stale timeout; if the number keeps growing, the transport keeps failing.",
    other: "{count} send jobs ran out of retries. Their recipients are re-queued automatically after the stale timeout; if the number keeps growing, the transport keeps failing."
  },
  pl: {
    one: "{count} zadaniu wysyłki skończyły się próby. Jego odbiorcy wracają do kolejki automatycznie po upływie limitu czasu; jeśli ta liczba wciąż rośnie, transport ciągle zawodzi.",
    few: "{count} zadaniom wysyłki skończyły się próby. Ich odbiorcy wracają do kolejki automatycznie po upływie limitu czasu; jeśli ta liczba wciąż rośnie, transport ciągle zawodzi.",
    many: "{count} zadaniom wysyłki skończyły się próby. Ich odbiorcy wracają do kolejki automatycznie po upływie limitu czasu; jeśli ta liczba wciąż rośnie, transport ciągle zawodzi.",
    other: "{count} zadania wysyłki wyczerpało próby. Ich odbiorcy wracają do kolejki automatycznie po upływie limitu czasu; jeśli ta liczba wciąż rośnie, transport ciągle zawodzi."
  },
  hi: {
    one: "{count} सेंड जॉब के सभी पुनः प्रयास खत्म हो गए। स्टेल टाइमआउट के बाद उसके प्राप्तकर्ता अपने-आप फिर से क्यू में डाले जाते हैं; अगर यह संख्या बढ़ती रहे, तो ट्रांसपोर्ट लगातार विफल हो रहा है।",
    other: "{count} सेंड जॉब के सभी पुनः प्रयास खत्म हो गए। स्टेल टाइमआउट के बाद उनके प्राप्तकर्ता अपने-आप फिर से क्यू में डाले जाते हैं; अगर यह संख्या बढ़ती रहे, तो ट्रांसपोर्ट लगातार विफल हो रहा है।"
  },
  de: {
    one: "{count} Versandauftrag hat alle Wiederholungen aufgebraucht. Seine Empfänger werden nach dem Stale-Timeout automatisch neu eingereiht; wächst die Zahl weiter, scheitert der Mailversand dauerhaft.",
    other: "{count} Versandaufträge haben alle Wiederholungen aufgebraucht. Ihre Empfänger werden nach dem Stale-Timeout automatisch neu eingereiht; wächst die Zahl weiter, scheitert der Mailversand dauerhaft."
  },
  fr: {
    one: "{count} tâche d'envoi a épuisé ses tentatives. Ses destinataires sont remis en file automatiquement après le délai d'expiration ; si ce nombre ne cesse d'augmenter, le transport échoue en permanence.",
    many: "{count} tâches d'envoi ont épuisé leurs tentatives. Leurs destinataires sont remis en file automatiquement après le délai d'expiration ; si ce nombre ne cesse d'augmenter, le transport échoue en permanence.",
    other: "{count} tâches d'envoi ont épuisé leurs tentatives. Leurs destinataires sont remis en file automatiquement après le délai d'expiration ; si ce nombre ne cesse d'augmenter, le transport échoue en permanence."
  }
}; // warn.deadLetters

export const PANEL_PROBLEMS = {
  no_transport: {
    en: "No mail transport is configured, so nothing can be sent — not campaigns, not test mails, not confirmations.",
    pl: "Nie skonfigurowano transportu poczty, więc nic nie zostanie wysłane — ani kampanie, ani maile testowe, ani potwierdzenia.",
    hi: "कोई मेल ट्रांसपोर्ट कॉन्फ़िगर नहीं है, इसलिए कुछ भी नहीं भेजा जा सकता — न कैंपेन, न टेस्ट ईमेल, न पुष्टि ईमेल।",
    de: "Kein Mailversand konfiguriert, daher kann nichts gesendet werden — keine Kampagnen, keine Testmails, keine Bestätigungen.",
    fr: "Aucun transport d'e-mail n'est configuré : rien ne peut être envoyé — ni campagnes, ni e-mails de test, ni confirmations."
  }, // warn.noTransport
  no_site_url: {
    en: "NEWSLETTER_SITE_URL is missing or not an https:// address. Every mail links to the site's unsubscribe page, so nothing can be sent without it.",
    pl: "Brakuje NEWSLETTER_SITE_URL albo nie jest to adres https://. Każdy mail zawiera link do strony wypisu, więc bez niego nic nie zostanie wysłane.",
    hi: "NEWSLETTER_SITE_URL मौजूद नहीं है या https:// पता नहीं है। हर ईमेल साइट के अनसब्सक्राइब पेज से लिंक होता है, इसलिए इसके बिना कुछ भी नहीं भेजा जा सकता।",
    de: "NEWSLETTER_SITE_URL fehlt oder ist keine https://-Adresse. Jede Mail verlinkt auf die Abmeldeseite der Website, ohne sie kann also nichts gesendet werden.",
    fr: "NEWSLETTER_SITE_URL est absent ou n'est pas une adresse https://. Chaque e-mail renvoie vers la page de désabonnement du site : sans lui, rien ne peut être envoyé."
  }, // warn.noSiteUrl
  no_sender: {
    en: "NEWSLETTER_MAIL_FROM is not set: each campaign then needs its own sender.",
    pl: "NEWSLETTER_MAIL_FROM nie jest ustawiony: każda kampania potrzebuje wtedy własnego nadawcy.",
    hi: "NEWSLETTER_MAIL_FROM सेट नहीं है: तब हर कैंपेन को अपना प्रेषक चाहिए।",
    de: "NEWSLETTER_MAIL_FROM ist nicht gesetzt: Jede Kampagne braucht dann einen eigenen Absender.",
    fr: "NEWSLETTER_MAIL_FROM n'est pas défini : chaque campagne doit alors avoir son propre expéditeur."
  }, // warn.noSender
  no_queue: {
    en: "The send queue is not bound (NEWSLETTER_QUEUE), so scheduled campaigns never go out.",
    pl: "Kolejka wysyłki nie jest podpięta (NEWSLETTER_QUEUE), więc zaplanowane kampanie nigdy nie wychodzą.",
    hi: "सेंड क्यू जुड़ी नहीं है (NEWSLETTER_QUEUE), इसलिए शेड्यूल किए गए कैंपेन कभी नहीं भेजे जाते।",
    de: "Die Versandwarteschlange ist nicht gebunden (NEWSLETTER_QUEUE), daher werden geplante Kampagnen nie versendet.",
    fr: "La file d'envoi n'est pas liée (NEWSLETTER_QUEUE) : les campagnes programmées ne partent jamais."
  }, // warn.noQueue
} satisfies Record<string, Text>;
