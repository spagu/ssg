// The words newsletter-ui itself puts into a campaign mail: the footer with the
// unsubscribe link. The campaign's own content is written by the admin in
// whatever language they choose; only this footer is localised, in the
// recipient's stored language.
//
// The languages match the `newsletter` template's pages (en, pl, hi, de, fr),
// so a reader meets the same language on the unsubscribe page the footer links
// to. To add one, copy the `en` block. Anything not shipped falls back to en.

export interface FooterStrings {
  reason: string; // why the reader gets this mail; {site} = the site's host name
  unsubscribe: string; // the link text
}

export const FOOTER: Record<string, FooterStrings> = {
  en: {
    reason: "You are receiving this email because this address subscribed to the newsletter at {site}.",
    unsubscribe: "Unsubscribe",
  },
  pl: {
    reason: "Otrzymujesz tę wiadomość, ponieważ ten adres został zapisany do newslettera na {site}.",
    unsubscribe: "Wypisz się",
  },
  hi: {
    reason: "आपको यह ईमेल इसलिए मिला है क्योंकि इस पते से {site} पर न्यूज़लेटर की सदस्यता ली गई थी।",
    unsubscribe: "सदस्यता छोड़ें",
  },
  de: {
    reason: "Sie erhalten diese E-Mail, weil diese Adresse den Newsletter auf {site} abonniert hat.",
    unsubscribe: "Abmelden",
  },
  fr: {
    reason: "Vous recevez cet e-mail parce que cette adresse est abonnée à la newsletter de {site}.",
    unsubscribe: "Se désabonner",
  },
};

// footerLanguage maps a stored language ("pl", "pt-BR", null) to a shipped
// footer language by its primary subtag, falling back to en.
export function footerLanguage(language: string | null | undefined): string {
  const primary = (language ?? "").split("-")[0].toLowerCase();
  return Object.hasOwn(FOOTER, primary) ? primary : "en";
}
