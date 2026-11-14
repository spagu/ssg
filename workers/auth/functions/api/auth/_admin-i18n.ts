// The words the admin panel's shell carries before its script runs (GO-138):
// the title, the description, the skip link, the menu, sign-out, the message
// without JavaScript and the refusal page. Everything else the panel says is
// in public/auth/assets/admin/js/strings/*.js. A leading underscore keeps
// this file out of the Pages route table; a test checks every language has
// every key and the same placeholders. Terms follow _i18n-*.ts and the
// newsletter panel's strings.

import type { Lang } from "./_i18n";

const EN = {
  title: "Accounts admin",
  description: "Administration of the accounts on {site}: people, invitations, groups, roles, sessions, service accounts and the audit log.",
  skip: "Skip to content",
  menu: "Menu",
  sections: "Accounts admin sections",
  preferences: "Preferences",
  signedInAs: "Signed in as {email}",
  signOut: "Sign out",
  noScript: "The accounts admin needs JavaScript. Without it, the admin API under /api/auth/admin/ still answers (see the worker's docs/API.md).",
  forbiddenTitle: "No access to the accounts admin",
  forbiddenText: "Your account {email} holds neither auth:admin nor auth:invite, which the accounts admin needs. Ask an administrator, or go back to your panels.",
  backToPanels: "Your panels",
};

export type AdminStrings = { readonly [K in keyof typeof EN]: string };

const PL: AdminStrings = {
  title: "Administracja kont",
  description: "Administracja kontami w {site}: osoby, zaproszenia, grupy, role, sesje, konta usług i dziennik zdarzeń.",
  skip: "Przejdź do treści",
  menu: "Menu",
  sections: "Sekcje administracji kont",
  preferences: "Preferencje",
  signedInAs: "Zalogowano jako {email}",
  signOut: "Wyloguj",
  noScript: "Administracja kont wymaga JavaScriptu. Bez niego API administracyjne pod /api/auth/admin/ nadal odpowiada (zobacz docs/API.md workera).",
  forbiddenTitle: "Brak dostępu do administracji kont",
  forbiddenText: "Twoje konto {email} nie ma ani auth:admin, ani auth:invite, których wymaga administracja kont. Poproś administratora albo wróć do swoich paneli.",
  backToPanels: "Twoje panele",
};

const HI: AdminStrings = {
  title: "खाता प्रशासन",
  description: "{site} पर खातों का प्रशासन: लोग, आमंत्रण, समूह, भूमिकाएँ, सेशन, सेवा खाते और ऑडिट लॉग।",
  skip: "सामग्री पर जाएँ",
  menu: "मेनू",
  sections: "खाता प्रशासन के भाग",
  preferences: "पसंद",
  signedInAs: "{email} के रूप में साइन इन",
  signOut: "साइन आउट",
  noScript: "खाता प्रशासन के लिए JavaScript चाहिए। इसके बिना भी /api/auth/admin/ के अंतर्गत प्रशासन API उत्तर देता है (वर्कर का docs/API.md देखें)।",
  forbiddenTitle: "खाता प्रशासन तक पहुँच नहीं",
  forbiddenText: "आपके खाते {email} के पास न auth:admin है और न auth:invite, जो खाता प्रशासन के लिए आवश्यक हैं। किसी प्रशासक से पूछें, या अपने पैनलों पर लौटें।",
  backToPanels: "आपके पैनल",
};

const DE: AdminStrings = {
  title: "Kontenverwaltung",
  description: "Verwaltung der Konten auf {site}: Personen, Einladungen, Gruppen, Rollen, Sitzungen, Dienstkonten und das Prüfprotokoll.",
  skip: "Zum Inhalt springen",
  menu: "Menü",
  sections: "Bereiche der Kontenverwaltung",
  preferences: "Einstellungen",
  signedInAs: "Angemeldet als {email}",
  signOut: "Abmelden",
  noScript: "Die Kontenverwaltung braucht JavaScript. Ohne JavaScript antwortet die Verwaltungs-API unter /api/auth/admin/ weiterhin (siehe docs/API.md des Workers).",
  forbiddenTitle: "Kein Zugang zur Kontenverwaltung",
  forbiddenText: "Ihr Konto {email} hat weder auth:admin noch auth:invite, die die Kontenverwaltung braucht. Fragen Sie eine Administratorin oder einen Administrator, oder kehren Sie zu Ihren Panels zurück.",
  backToPanels: "Ihre Panels",
};

const FR: AdminStrings = {
  title: "Administration des comptes",
  description: "Administration des comptes de {site} : personnes, invitations, groupes, rôles, sessions, comptes de service et journal d'audit.",
  skip: "Aller au contenu",
  menu: "Menu",
  sections: "Sections de l'administration des comptes",
  preferences: "Préférences",
  signedInAs: "Connecté en tant que {email}",
  signOut: "Se déconnecter",
  noScript: "L'administration des comptes a besoin de JavaScript. Sans lui, l'API d'administration sous /api/auth/admin/ répond toujours (voir docs/API.md du worker).",
  forbiddenTitle: "Pas d'accès à l'administration des comptes",
  forbiddenText: "Votre compte {email} n'a ni auth:admin ni auth:invite, nécessaires à l'administration des comptes. Demandez à un administrateur, ou revenez à vos panneaux.",
  backToPanels: "Vos panneaux",
};

export const ADMIN_STRINGS: Record<Lang, AdminStrings> = { en: EN, pl: PL, hi: HI, de: DE, fr: FR };
