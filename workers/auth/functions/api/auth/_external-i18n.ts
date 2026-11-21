// The words of the external JWT issuer's parts of the pages (#435). A leading
// underscore keeps this file out of the Pages route table. Kept apart from
// _i18n-*.ts so the method can be read (and removed) as one piece, like
// _access-i18n.ts. {issuer} is the issuer as configured (AUTH_JWT_ISSUER), the
// name the account page lists the identity under.
//
// The pages' strings are in all five languages (a test checks every key, real
// text and the same placeholders).

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
