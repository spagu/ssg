// Server-side strings: the confirm / unsubscribe pages, the error page a plain
// form post can land on, and the confirmation mail. A leading underscore keeps
// this file out of the Pages route table — it is imported, never served.
//
// To add a language, copy the `en` block. Anything not shipped falls back to en.

export interface Strings {
  unsubTitle: string;
  unsubAsk: string;
  unsubButton: string;
  unsubDone: string;
  confirmTitle: string;
  confirmAsk: string;
  confirmButton: string;
  confirmDone: string;
  invalid: string;
  errorTitle: string;
  invalid_email: string;
  consent_required: string;
  captcha_failed: string;
  not_configured: string;
  back: string;
  mailSubject: string;
  mailBody: string; // {url} is replaced with the confirmation link
}

export const STRINGS: Record<string, Strings> = {
  en: {
    unsubTitle: "Unsubscribe",
    unsubAsk: "Stop sending me the newsletter?",
    unsubButton: "Unsubscribe",
    unsubDone: "You are unsubscribed. No further newsletters will be sent to you.",
    confirmTitle: "Confirm your subscription",
    confirmAsk: "Please confirm that you want to receive the newsletter.",
    confirmButton: "Confirm subscription",
    confirmDone: "Thank you — your subscription is confirmed.",
    invalid: "This link is not valid any more. It may have expired or already been used.",
    errorTitle: "Sign-up not completed",
    invalid_email: "Please go back and enter a valid email address.",
    consent_required: "Please go back and tick the consent box to subscribe.",
    captcha_failed: "The anti-spam check did not pass. Please go back and try again.",
    not_configured: "Newsletter sign-up is not available at the moment.",
    back: "Back to the site",
    mailSubject: "Confirm your newsletter subscription",
    mailBody:
      "Someone — hopefully you — asked to receive our newsletter at this address.\n\n" +
      "To confirm, open this link:\n{url}\n\n" +
      "If it was not you, ignore this email: without confirmation nothing will be sent.",
  },
  pl: {
    unsubTitle: "Wypisz się",
    unsubAsk: "Czy na pewno nie chcesz już otrzymywać newslettera?",
    unsubButton: "Wypisz mnie",
    unsubDone: "Wypisano Cię z newslettera. Nie wyślemy Ci kolejnych wiadomości.",
    confirmTitle: "Potwierdź subskrypcję",
    confirmAsk: "Potwierdź, że chcesz otrzymywać newsletter.",
    confirmButton: "Potwierdzam subskrypcję",
    confirmDone: "Dziękujemy — subskrypcja została potwierdzona.",
    invalid: "Ten link jest już nieaktualny. Mógł wygasnąć albo został już użyty.",
    errorTitle: "Zapis nie powiódł się",
    invalid_email: "Wróć i wpisz poprawny adres e-mail.",
    consent_required: "Wróć i zaznacz zgodę, aby się zapisać.",
    captcha_failed: "Weryfikacja antyspamowa nie powiodła się. Wróć i spróbuj ponownie.",
    not_configured: "Zapis do newslettera jest chwilowo niedostępny.",
    back: "Wróć na stronę",
    mailSubject: "Potwierdź zapis do newslettera",
    mailBody:
      "Ktoś — mamy nadzieję, że Ty — poprosił o newsletter na ten adres.\n\n" +
      "Aby potwierdzić, otwórz link:\n{url}\n\n" +
      "Jeśli to nie Ty, zignoruj tę wiadomość: bez potwierdzenia nic nie wyślemy.",
  },
  hi: {
    unsubTitle: "सदस्यता छोड़ें",
    unsubAsk: "क्या आप न्यूज़लेटर पाना बंद करना चाहते हैं?",
    unsubButton: "सदस्यता छोड़ें",
    unsubDone: "आपकी सदस्यता समाप्त कर दी गई है। आपको आगे कोई न्यूज़लेटर नहीं भेजा जाएगा।",
    confirmTitle: "अपनी सदस्यता की पुष्टि करें",
    confirmAsk: "कृपया पुष्टि करें कि आप न्यूज़लेटर पाना चाहते हैं।",
    confirmButton: "सदस्यता की पुष्टि करें",
    confirmDone: "धन्यवाद — आपकी सदस्यता की पुष्टि हो गई है।",
    invalid: "यह लिंक अब मान्य नहीं है। हो सकता है इसकी अवधि समाप्त हो गई हो या इसका पहले ही उपयोग हो चुका हो।",
    errorTitle: "सदस्यता पूरी नहीं हुई",
    invalid_email: "कृपया वापस जाकर एक मान्य ईमेल पता दर्ज करें।",
    consent_required: "सदस्यता लेने के लिए कृपया वापस जाकर सहमति वाले बॉक्स पर निशान लगाएँ।",
    captcha_failed: "स्पैम-रोधी जाँच सफल नहीं हुई। कृपया वापस जाकर फिर से प्रयास करें।",
    not_configured: "न्यूज़लेटर की सदस्यता अभी उपलब्ध नहीं है।",
    back: "साइट पर वापस जाएँ",
    mailSubject: "अपनी न्यूज़लेटर सदस्यता की पुष्टि करें",
    mailBody:
      "किसी ने — उम्मीद है आपने — इस पते पर हमारा न्यूज़लेटर पाने का अनुरोध किया है।\n\n" +
      "पुष्टि करने के लिए यह लिंक खोलें:\n{url}\n\n" +
      "अगर यह आप नहीं थे, तो इस ईमेल को अनदेखा करें: पुष्टि के बिना कुछ नहीं भेजा जाएगा।",
  },
  de: {
    unsubTitle: "Abmelden",
    unsubAsk: "Möchten Sie den Newsletter abbestellen?",
    unsubButton: "Abmelden",
    unsubDone: "Sie sind abgemeldet. Sie erhalten keine weiteren Newsletter.",
    confirmTitle: "Anmeldung bestätigen",
    confirmAsk: "Bitte bestätigen Sie, dass Sie den Newsletter erhalten möchten.",
    confirmButton: "Anmeldung bestätigen",
    confirmDone: "Vielen Dank — Ihre Anmeldung ist bestätigt.",
    invalid: "Dieser Link ist nicht mehr gültig. Er ist abgelaufen oder wurde bereits verwendet.",
    errorTitle: "Anmeldung nicht abgeschlossen",
    invalid_email: "Bitte gehen Sie zurück und geben Sie eine gültige E-Mail-Adresse ein.",
    consent_required: "Bitte gehen Sie zurück und setzen Sie das Häkchen bei der Einwilligung.",
    captcha_failed: "Die Spam-Prüfung ist fehlgeschlagen. Bitte gehen Sie zurück und versuchen Sie es erneut.",
    not_configured: "Die Newsletter-Anmeldung ist derzeit nicht verfügbar.",
    back: "Zurück zur Website",
    mailSubject: "Bitte bestätigen Sie Ihre Newsletter-Anmeldung",
    mailBody:
      "Jemand — hoffentlich Sie — hat den Newsletter für diese Adresse angefordert.\n\n" +
      "Zur Bestätigung öffnen Sie diesen Link:\n{url}\n\n" +
      "Falls Sie das nicht waren, ignorieren Sie diese E-Mail: Ohne Bestätigung wird nichts versendet.",
  },
  fr: {
    unsubTitle: "Se désabonner",
    unsubAsk: "Ne plus recevoir la newsletter ?",
    unsubButton: "Me désabonner",
    unsubDone: "Vous êtes désabonné. Vous ne recevrez plus la newsletter.",
    confirmTitle: "Confirmez votre inscription",
    confirmAsk: "Merci de confirmer que vous souhaitez recevoir la newsletter.",
    confirmButton: "Confirmer l'inscription",
    confirmDone: "Merci — votre inscription est confirmée.",
    invalid: "Ce lien n'est plus valable. Il a peut-être expiré ou a déjà été utilisé.",
    errorTitle: "Inscription non terminée",
    invalid_email: "Revenez en arrière et saisissez une adresse e-mail valide.",
    consent_required: "Revenez en arrière et cochez la case de consentement pour vous inscrire.",
    captcha_failed: "La vérification anti-spam a échoué. Revenez en arrière et réessayez.",
    not_configured: "L'inscription à la newsletter est momentanément indisponible.",
    back: "Retour au site",
    mailSubject: "Confirmez votre inscription à la newsletter",
    mailBody:
      "Quelqu'un — vous, nous l'espérons — a demandé à recevoir notre newsletter à cette adresse.\n\n" +
      "Pour confirmer, ouvrez ce lien :\n{url}\n\n" +
      "Si ce n'était pas vous, ignorez cet e-mail : sans confirmation, rien ne sera envoyé.",
  },
};

// pickLang returns the first supported language among the candidates — a
// subscriber's stored language, a form field — then the Accept-Language
// header, then English.
export function pickLang(request: Request, ...candidates: (string | null | undefined)[]): string {
  const accept = (request.headers.get("accept-language") || "")
    .split(",")
    .map((part) => part.split(";")[0].trim());
  for (const c of [...candidates, ...accept]) {
    const primary = (c || "").split("-")[0].toLowerCase();
    if (STRINGS[primary]) return primary;
  }
  return "en";
}
