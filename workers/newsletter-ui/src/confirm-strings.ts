// The double opt-in confirmation mail, word for word as the `newsletter`
// template sends it (functions/api/newsletter/_i18n.ts, mailSubject and
// mailBody). A scaffolded newsletter-ui stands alone and cannot import from
// the sibling template, so the strings are copied; test/confirm-copies.test.ts
// fails when the two copies drift. To change the wording, change the
// template's _i18n.ts first, then copy it here.

export interface ConfirmStrings {
  mailSubject: string;
  mailBody: string; // {url} is replaced with the confirmation link
}

export const CONFIRM_STRINGS: Record<string, ConfirmStrings> = {
  en: {
    mailSubject: "Confirm your newsletter subscription",
    mailBody:
      "Someone — hopefully you — asked to receive our newsletter at this address.\n\n" +
      "To confirm, open this link:\n{url}\n\n" +
      "If it was not you, ignore this email: without confirmation nothing will be sent.",
  },
  pl: {
    mailSubject: "Potwierdź zapis do newslettera",
    mailBody:
      "Ktoś — mamy nadzieję, że Ty — poprosił o newsletter na ten adres.\n\n" +
      "Aby potwierdzić, otwórz link:\n{url}\n\n" +
      "Jeśli to nie Ty, zignoruj tę wiadomość: bez potwierdzenia nic nie wyślemy.",
  },
  hi: {
    mailSubject: "अपनी न्यूज़लेटर सदस्यता की पुष्टि करें",
    mailBody:
      "किसी ने — उम्मीद है आपने — इस पते पर हमारा न्यूज़लेटर पाने का अनुरोध किया है।\n\n" +
      "पुष्टि करने के लिए यह लिंक खोलें:\n{url}\n\n" +
      "अगर यह आप नहीं थे, तो इस ईमेल को अनदेखा करें: पुष्टि के बिना कुछ नहीं भेजा जाएगा।",
  },
  de: {
    mailSubject: "Bitte bestätigen Sie Ihre Newsletter-Anmeldung",
    mailBody:
      "Jemand — hoffentlich Sie — hat den Newsletter für diese Adresse angefordert.\n\n" +
      "Zur Bestätigung öffnen Sie diesen Link:\n{url}\n\n" +
      "Falls Sie das nicht waren, ignorieren Sie diese E-Mail: Ohne Bestätigung wird nichts versendet.",
  },
  fr: {
    mailSubject: "Confirmez votre inscription à la newsletter",
    mailBody:
      "Quelqu'un — vous, nous l'espérons — a demandé à recevoir notre newsletter à cette adresse.\n\n" +
      "Pour confirmer, ouvrez ce lien :\n{url}\n\n" +
      "Si ce n'était pas vous, ignorez cet e-mail : sans confirmation, rien ne sera envoyé.",
  },
};
