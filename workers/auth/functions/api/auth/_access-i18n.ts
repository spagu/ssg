// The words of the Cloudflare Access parts of the pages (GO-139), in the five
// languages of the pages. A leading underscore keeps this file out of the
// Pages route table. Kept apart from _i18n-*.ts so the method can be read (and
// removed) as one piece; a test checks every language has every key, real
// text and the same placeholders. "Cloudflare Access" is a product name and
// stays as it is. Terms follow _i18n-*.ts (sign in, connect, account).

import type { Lang } from "./_i18n";

const EN = {
  signIn: "Sign in with Cloudflare Access",
  connectButton: "Connect Cloudflare Access",
  confirmTitle: "Connect Cloudflare Access",
  confirmDescription: "Connect Cloudflare Access to your account at {site}.",
  confirmLead: "Cloudflare Access says you are {email}. Connect this identity to your account {account}? From then on you can sign in here through Cloudflare Access.",
  confirmButton: "Connect",
  cancel: "Cancel",
  errFailed: "Cloudflare Access did not vouch for this sign-in, so you are not signed in. Open the sign-in again; if it keeps happening, tell the site's administrator.",
  errUnavailable: "Cloudflare Access could not be reached to check the sign-in. Try again in a few minutes.",
  errNotLinked: "An account with this address exists here, but Cloudflare Access is not connected to it. Sign in another way, then connect Cloudflare Access on your account page.",
  errEmailUnusable: "Cloudflare Access gave an address that cannot be matched to an account here (it is missing or uses characters outside plain ASCII). Ask the site's administrator.",
};

export type AccessStrings = { readonly [K in keyof typeof EN]: string };

const PL: AccessStrings = {
  signIn: "Zaloguj się przez Cloudflare Access",
  connectButton: "Połącz Cloudflare Access",
  confirmTitle: "Połącz Cloudflare Access",
  confirmDescription: "Połącz Cloudflare Access ze swoim kontem w {site}.",
  confirmLead: "Cloudflare Access potwierdza, że jesteś {email}. Połączyć tę tożsamość z kontem {account}? Od tej chwili możesz się tu logować przez Cloudflare Access.",
  confirmButton: "Połącz",
  cancel: "Anuluj",
  errFailed: "Cloudflare Access nie potwierdził tego logowania, więc logowanie się nie powiodło. Otwórz logowanie ponownie; jeśli to się powtarza, powiedz administratorowi strony.",
  errUnavailable: "Nie udało się połączyć z Cloudflare Access, żeby sprawdzić logowanie. Spróbuj ponownie za kilka minut.",
  errNotLinked: "Konto z tym adresem już tu istnieje, ale Cloudflare Access nie jest z nim połączony. Zaloguj się w inny sposób, a potem połącz Cloudflare Access na stronie swojego konta.",
  errEmailUnusable: "Cloudflare Access podał adres, którego nie można dopasować do konta tutaj (brak adresu albo znaki spoza zwykłego ASCII). Poproś administratora strony o pomoc.",
};

const HI: AccessStrings = {
  signIn: "Cloudflare Access से साइन इन करें",
  connectButton: "Cloudflare Access जोड़ें",
  confirmTitle: "Cloudflare Access जोड़ें",
  confirmDescription: "{site} पर अपने खाते से Cloudflare Access जोड़ें।",
  confirmLead: "Cloudflare Access के अनुसार आप {email} हैं। क्या इस पहचान को अपने खाते {account} से जोड़ना है? इसके बाद आप यहाँ Cloudflare Access से साइन इन कर सकेंगे।",
  confirmButton: "जोड़ें",
  cancel: "रद्द करें",
  errFailed: "Cloudflare Access ने इस साइन-इन की पुष्टि नहीं की, इसलिए आप साइन इन नहीं हुए। साइन-इन फिर से खोलें; अगर ऐसा बार-बार हो, तो साइट के एडमिनिस्ट्रेटर को बताएँ।",
  errUnavailable: "साइन-इन जाँचने के लिए Cloudflare Access से संपर्क नहीं हो सका। कुछ मिनट बाद फिर से कोशिश करें।",
  errNotLinked: "इस पते वाला खाता यहाँ पहले से है, लेकिन Cloudflare Access उससे जुड़ा नहीं है। किसी दूसरे तरीके से साइन इन करें, फिर अपने खाते के पेज पर Cloudflare Access जोड़ें।",
  errEmailUnusable: "Cloudflare Access ने ऐसा पता दिया जो यहाँ किसी खाते से मिलाया नहीं जा सकता (पता नहीं है या उसमें सामान्य ASCII के बाहर के अक्षर हैं)। साइट के एडमिनिस्ट्रेटर से पूछें।",
};

const DE: AccessStrings = {
  signIn: "Mit Cloudflare Access anmelden",
  connectButton: "Cloudflare Access verbinden",
  confirmTitle: "Cloudflare Access verbinden",
  confirmDescription: "Verbinden Sie Cloudflare Access mit Ihrem Konto bei {site}.",
  confirmLead: "Laut Cloudflare Access sind Sie {email}. Diese Identität mit Ihrem Konto {account} verbinden? Danach können Sie sich hier über Cloudflare Access anmelden.",
  confirmButton: "Verbinden",
  cancel: "Abbrechen",
  errFailed: "Cloudflare Access hat diese Anmeldung nicht bestätigt, daher sind Sie nicht angemeldet. Öffnen Sie die Anmeldung erneut; wenn es wieder passiert, sagen Sie es der Administration der Website.",
  errUnavailable: "Cloudflare Access war nicht erreichbar, um die Anmeldung zu prüfen. Versuchen Sie es in einigen Minuten erneut.",
  errNotLinked: "Hier gibt es schon ein Konto mit dieser Adresse, aber Cloudflare Access ist nicht damit verbunden. Melden Sie sich auf andere Weise an und verbinden Sie Cloudflare Access dann auf Ihrer Kontoseite.",
  errEmailUnusable: "Cloudflare Access hat eine Adresse geliefert, die hier keinem Konto zugeordnet werden kann (sie fehlt oder enthält Zeichen außerhalb von einfachem ASCII). Wenden Sie sich an die Administration der Website.",
};

const FR: AccessStrings = {
  signIn: "Se connecter avec Cloudflare Access",
  connectButton: "Relier Cloudflare Access",
  confirmTitle: "Relier Cloudflare Access",
  confirmDescription: "Reliez Cloudflare Access à votre compte sur {site}.",
  confirmLead: "Selon Cloudflare Access, vous êtes {email}. Relier cette identité à votre compte {account} ? Vous pourrez ensuite vous connecter ici avec Cloudflare Access.",
  confirmButton: "Relier",
  cancel: "Annuler",
  errFailed: "Cloudflare Access n'a pas confirmé cette connexion, vous n'êtes donc pas connecté. Relancez la connexion ; si cela se répète, prévenez l'administrateur du site.",
  errUnavailable: "Cloudflare Access n'a pas pu être joint pour vérifier la connexion. Réessayez dans quelques minutes.",
  errNotLinked: "Un compte avec cette adresse existe déjà ici, mais Cloudflare Access n'y est pas relié. Connectez-vous autrement, puis reliez Cloudflare Access depuis la page de votre compte.",
  errEmailUnusable: "Cloudflare Access a fourni une adresse qui ne peut être associée à aucun compte ici (absente, ou avec des caractères hors ASCII simple). Demandez à l'administrateur du site.",
};

export const ACCESS_STRINGS: Record<Lang, AccessStrings> = { en: EN, pl: PL, hi: HI, de: DE, fr: FR };

// The ?error= codes only an Access sign-in sends, and their sentence.
const ERRORS: Record<string, keyof AccessStrings> = {
  access_failed: "errFailed",
  access_unavailable: "errUnavailable",
  access_not_linked: "errNotLinked",
  access_email_unusable: "errEmailUnusable",
};

// accessErrorText: the sentence for an Access error code; "" for any other
// code (the pages' own list, _pages-errors.ts, takes those).
export function accessErrorText(lang: Lang, code: string | null): string {
  return code && Object.hasOwn(ERRORS, code) ? ACCESS_STRINGS[lang][ERRORS[code]] : "";
}
