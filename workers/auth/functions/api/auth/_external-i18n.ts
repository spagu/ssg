// The words of the external JWT issuer's parts of the pages (#435). A leading
// underscore keeps this file out of the Pages route table. Kept apart from
// _i18n-*.ts so the method can be read (and removed) as one piece, like
// _access-i18n.ts. {issuer} is the issuer as configured (AUTH_JWT_ISSUER), the
// name the account page lists the identity under.
//
// The pages' strings are in all five languages (a test checks every key, real
// text and the same placeholders). The sentences for the method's own ?error=
// codes are in English and Polish only; the other languages show the pages'
// generic sentence for them (_pages-errors.ts).

import type { Lang } from "./_i18n";

const EN = {
  connectButton: "Connect {issuer}",
  confirmTitle: "Connect your sign-in from {issuer}",
  confirmDescription: "Connect your sign-in from {issuer} to your account at {site}.",
  confirmLead: "{issuer} says you are {email}. Connect this identity to your account {account}? From then on you can sign in here through {issuer}.",
  confirmButton: "Connect",
  cancel: "Cancel",
};

export type ExternalStrings = { readonly [K in keyof typeof EN]: string };

const PL: ExternalStrings = {
  connectButton: "Połącz {issuer}",
  confirmTitle: "Połącz logowanie z {issuer}",
  confirmDescription: "Połącz logowanie z {issuer} ze swoim kontem w {site}.",
  confirmLead: "{issuer} potwierdza, że jesteś {email}. Połączyć tę tożsamość z kontem {account}? Od tej chwili możesz się tu logować przez {issuer}.",
  confirmButton: "Połącz",
  cancel: "Anuluj",
};

const HI: ExternalStrings = {
  connectButton: "{issuer} जोड़ें",
  confirmTitle: "{issuer} से साइन-इन जोड़ें",
  confirmDescription: "{site} पर अपने खाते से {issuer} का साइन-इन जोड़ें।",
  confirmLead: "{issuer} के अनुसार आप {email} हैं। क्या इस पहचान को अपने खाते {account} से जोड़ना है? इसके बाद आप यहाँ {issuer} से साइन इन कर सकेंगे।",
  confirmButton: "जोड़ें",
  cancel: "रद्द करें",
};

const DE: ExternalStrings = {
  connectButton: "{issuer} verbinden",
  confirmTitle: "Anmeldung über {issuer} verbinden",
  confirmDescription: "Verbinden Sie die Anmeldung über {issuer} mit Ihrem Konto bei {site}.",
  confirmLead: "Laut {issuer} sind Sie {email}. Diese Identität mit Ihrem Konto {account} verbinden? Danach können Sie sich hier über {issuer} anmelden.",
  confirmButton: "Verbinden",
  cancel: "Abbrechen",
};

const FR: ExternalStrings = {
  connectButton: "Relier {issuer}",
  confirmTitle: "Relier la connexion via {issuer}",
  confirmDescription: "Reliez la connexion via {issuer} à votre compte sur {site}.",
  confirmLead: "Selon {issuer}, vous êtes {email}. Relier cette identité à votre compte {account} ? Vous pourrez ensuite vous connecter ici via {issuer}.",
  confirmButton: "Relier",
  cancel: "Annuler",
};

export const EXTERNAL_STRINGS: Record<Lang, ExternalStrings> = { en: EN, pl: PL, hi: HI, de: DE, fr: FR };

// The ?error= codes only the external issuer's routes send, and their sentence.
const ERRORS_EN: Record<string, string> = {
  jwt_failed: "The sign-in your login system sent could not be checked (it was not valid, too old or already used), so you are not signed in. Start the sign-in again from there; if it keeps happening, tell the site's administrator.",
  jwt_unavailable: "The keys of your login system could not be fetched to check the sign-in. Try again in a few minutes.",
  jwt_not_linked: "An account with this address exists here, but your login system is not connected to it. Sign in another way, then connect it on your account page.",
  jwt_email_unusable: "Your login system gave no address that can be matched to an account here (it is missing, not verified, or uses characters outside plain ASCII). Ask the site's administrator.",
  jwt_link_expired: "The connection waited too long or was already used. Start it again from your login system.",
};

const ERRORS_PL: Record<string, string> = {
  jwt_failed: "Nie udało się sprawdzić logowania przysłanego przez Twój system logowania (było nieważne, za stare albo już użyte), więc logowanie się nie powiodło. Zacznij logowanie od nowa w tamtym systemie; jeśli to się powtarza, powiedz administratorowi strony.",
  jwt_unavailable: "Nie udało się pobrać kluczy Twojego systemu logowania, żeby sprawdzić logowanie. Spróbuj ponownie za kilka minut.",
  jwt_not_linked: "Konto z tym adresem już tu istnieje, ale Twój system logowania nie jest z nim połączony. Zaloguj się w inny sposób, a potem połącz go na stronie swojego konta.",
  jwt_email_unusable: "Twój system logowania nie podał adresu, który można dopasować do konta tutaj (brak adresu, adres niepotwierdzony albo znaki spoza zwykłego ASCII). Poproś administratora strony o pomoc.",
  jwt_link_expired: "Łączenie czekało za długo albo zostało już użyte. Zacznij je od nowa w swoim systemie logowania.",
};

const ERRORS: Partial<Record<Lang, Record<string, string>>> = { en: ERRORS_EN, pl: ERRORS_PL };

// externalErrorText: the sentence for one of the method's error codes in
// English or Polish; "" for any other code or language (the pages' own list
// takes those).
export function externalErrorText(lang: Lang, code: string | null): string {
  const list = ERRORS[lang];
  return list && code && Object.hasOwn(list, code) ? list[code] : "";
}
