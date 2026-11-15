// The summary's sentences in the five panel languages. Fixed sentences: the
// only value written into one is the branch name, which validBranch has
// limited to letters, digits and ._/- (no person's data ever).

import type { Text } from "./summary-kit";

export const LABELS: Record<"overlaySettings" | "themeFiles", Text> = {
  overlaySettings: {
    en: "Settings changed in the panel",
    pl: "Ustawienia zmienione w panelu",
    hi: "पैनल में बदली गई सेटिंग",
    de: "Im Panel geänderte Einstellungen",
    fr: "Réglages modifiés dans le panneau",
  },
  themeFiles: {
    en: "Theme files",
    pl: "Pliki motywu",
    hi: "थीम फ़ाइलें",
    de: "Theme-Dateien",
    fr: "Fichiers du thème",
  },
};

// BRANCH: an info item; {branch} is replaced.
export const BRANCH: Text = {
  en: "Publications are committed to the branch {branch}.",
  pl: "Publikacje trafiają jako commity na gałąź {branch}.",
  hi: "प्रकाशन ब्रांच {branch} पर कमिट किए जाते हैं।",
  de: "Veröffentlichungen werden als Commits auf den Branch {branch} geschrieben.",
  fr: "Les publications sont commitées sur la branche {branch}.",
};

export const PROBLEMS: Record<string, Text> = {
  repo_not_configured: {
    en: "The repository is not configured: DESIGNER_GIT_REPO or the DESIGNER_GIT_TOKEN secret is missing or malformed. The health route says which.",
    pl: "Repozytorium nie jest skonfigurowane: brakuje DESIGNER_GIT_REPO albo sekretu DESIGNER_GIT_TOKEN lub mają zły format. Trasa health mówi, której.",
    hi: "रिपॉज़िटरी कॉन्फ़िगर नहीं है: DESIGNER_GIT_REPO या सीक्रेट DESIGNER_GIT_TOKEN नहीं है या उसका रूप गलत है। health रूट बताता है कि कौन-सा।",
    de: "Das Repository ist nicht konfiguriert: DESIGNER_GIT_REPO oder das Secret DESIGNER_GIT_TOKEN fehlt oder ist fehlerhaft. Die Health-Route sagt, welches.",
    fr: "Le dépôt n'est pas configuré : DESIGNER_GIT_REPO ou le secret DESIGNER_GIT_TOKEN manque ou est mal formé. La route health indique lequel.",
  },
  repo_token_rejected: {
    en: "The repository host refuses the token DESIGNER_GIT_TOKEN: it has expired, was revoked, or lacks access to this repository. Nothing can be read or published.",
    pl: "Dostawca repozytorium odrzuca token DESIGNER_GIT_TOKEN: wygasł, został unieważniony albo nie ma dostępu do tego repozytorium. Niczego nie da się odczytać ani opublikować.",
    hi: "रिपॉज़िटरी होस्ट टोकन DESIGNER_GIT_TOKEN को अस्वीकार करता है: उसकी अवधि खत्म हो गई है, उसे रद्द किया गया है, या उसके पास इस रिपॉज़िटरी की पहुँच नहीं है। कुछ भी पढ़ा या प्रकाशित नहीं किया जा सकता।",
    de: "Der Repository-Anbieter lehnt das Token DESIGNER_GIT_TOKEN ab: Es ist abgelaufen, wurde widerrufen oder hat keinen Zugriff auf dieses Repository. Nichts kann gelesen oder veröffentlicht werden.",
    fr: "L'hébergeur du dépôt refuse le jeton DESIGNER_GIT_TOKEN : il a expiré, a été révoqué ou n'a pas accès à ce dépôt. Rien ne peut être lu ni publié.",
  },
  repo_not_found: {
    en: "The repository or the branch in DESIGNER_GIT_REPO / DESIGNER_GIT_BRANCH does not exist, or the token cannot see it.",
    pl: "Repozytorium albo gałąź z DESIGNER_GIT_REPO / DESIGNER_GIT_BRANCH nie istnieje albo token jej nie widzi.",
    hi: "DESIGNER_GIT_REPO / DESIGNER_GIT_BRANCH में दी गई रिपॉज़िटरी या ब्रांच मौजूद नहीं है, या टोकन उसे देख नहीं सकता।",
    de: "Das Repository oder der Branch aus DESIGNER_GIT_REPO / DESIGNER_GIT_BRANCH existiert nicht, oder das Token sieht ihn nicht.",
    fr: "Le dépôt ou la branche indiqués dans DESIGNER_GIT_REPO / DESIGNER_GIT_BRANCH n'existent pas, ou le jeton ne les voit pas.",
  },
  repo_unreachable: {
    en: "The repository host did not answer as expected (unavailable or rate-limited). Try again later.",
    pl: "Dostawca repozytorium nie odpowiedział zgodnie z oczekiwaniem (niedostępny albo przekroczony limit zapytań). Spróbuj ponownie później.",
    hi: "रिपॉज़िटरी होस्ट ने अपेक्षा के अनुसार जवाब नहीं दिया (उपलब्ध नहीं है या अनुरोधों की सीमा पूरी हो गई)। बाद में फिर कोशिश करें।",
    de: "Der Repository-Anbieter hat nicht wie erwartet geantwortet (nicht erreichbar oder Anfragelimit erreicht). Versuchen Sie es später erneut.",
    fr: "L'hébergeur du dépôt n'a pas répondu comme prévu (indisponible ou limite de requêtes atteinte). Réessayez plus tard.",
  },
  config_unreadable: {
    en: "The site's configuration file on the branch is missing or cannot be read by the designer, so settings cannot be published (they need a YAML configuration to sit beside).",
    pl: "Pliku konfiguracji strony na gałęzi nie ma albo projektant nie potrafi go odczytać, więc nie da się publikować ustawień (wymagają konfiguracji YAML, obok której leżą).",
    hi: "ब्रांच पर साइट की कॉन्फ़िगरेशन फ़ाइल नहीं है या डिज़ाइनर उसे पढ़ नहीं सकता, इसलिए सेटिंग प्रकाशित नहीं की जा सकतीं (उन्हें साथ रखने के लिए YAML कॉन्फ़िगरेशन चाहिए)।",
    de: "Die Konfigurationsdatei der Website fehlt auf dem Branch oder der Designer kann sie nicht lesen; Einstellungen lassen sich daher nicht veröffentlichen (sie brauchen eine YAML-Konfiguration, neben der sie liegen).",
    fr: "Le fichier de configuration du site manque sur la branche ou le concepteur ne peut pas le lire : les réglages ne peuvent donc pas être publiés (ils ont besoin d'une configuration YAML à côté de laquelle se placer).",
  },
  config_partly_read: {
    en: "Some settings in the site's configuration file could not be read by the designer; the settings tab marks them as set in the file, value not shown.",
    pl: "Projektant nie potrafił odczytać części ustawień z pliku konfiguracji strony; zakładka ustawień oznacza je jako ustawione w pliku, bez pokazanej wartości.",
    hi: "डिज़ाइनर साइट की कॉन्फ़िगरेशन फ़ाइल की कुछ सेटिंग नहीं पढ़ सका; सेटिंग टैब उन्हें फ़ाइल में सेट, मान दिखाया नहीं गया, के रूप में चिह्नित करता है।",
    de: "Einige Einstellungen der Konfigurationsdatei der Website konnte der Designer nicht lesen; der Einstellungs-Tab markiert sie als in der Datei gesetzt, Wert nicht angezeigt.",
    fr: "Le concepteur n'a pas pu lire certains réglages du fichier de configuration du site ; l'onglet des réglages les marque comme définis dans le fichier, valeur non affichée.",
  },
  overlay_invalid: {
    en: "The settings file .ssg.designer.json on the branch is not valid: the site's build stops until it is fixed or removed in the repository.",
    pl: "Plik ustawień .ssg.designer.json na gałęzi jest nieprawidłowy: build strony zatrzyma się, dopóki nie zostanie poprawiony albo usunięty w repozytorium.",
    hi: "ब्रांच पर सेटिंग फ़ाइल .ssg.designer.json मान्य नहीं है: जब तक उसे रिपॉज़िटरी में ठीक या हटाया नहीं जाता, साइट का बिल्ड रुका रहेगा।",
    de: "Die Einstellungsdatei .ssg.designer.json auf dem Branch ist ungültig: Der Build der Website bricht ab, bis sie im Repository korrigiert oder entfernt wird.",
    fr: "Le fichier de réglages .ssg.designer.json sur la branche n'est pas valide : le build du site s'arrête tant qu'il n'est pas corrigé ou supprimé dans le dépôt.",
  },
  theme_not_editable: {
    en: "The theme's files cannot be edited here: the theme is not in the repository (built into ssg or downloaded at build time), or the configuration does not name it. The theme list says how to bring it in; the settings work as usual.",
    pl: "Plików motywu nie da się tu edytować: motywu nie ma w repozytorium (jest wbudowany w ssg albo pobierany przy buildzie) albo konfiguracja go nie wskazuje. Lista motywu mówi, jak go przenieść do repozytorium; ustawienia działają normalnie.",
    hi: "थीम की फ़ाइलें यहाँ संपादित नहीं की जा सकतीं: थीम रिपॉज़िटरी में नहीं है (ssg में बनी है या बिल्ड के समय डाउनलोड होती है), या कॉन्फ़िगरेशन उसका नाम नहीं बताता। थीम सूची बताती है कि उसे रिपॉज़िटरी में कैसे लाएँ; सेटिंग सामान्य रूप से काम करती हैं।",
    de: "Die Dateien des Themes lassen sich hier nicht bearbeiten: Das Theme ist nicht im Repository (in ssg eingebaut oder beim Build heruntergeladen), oder die Konfiguration nennt es nicht. Die Theme-Liste sagt, wie es ins Repository kommt; die Einstellungen funktionieren wie gewohnt.",
    fr: "Les fichiers du thème ne peuvent pas être modifiés ici : le thème n'est pas dans le dépôt (intégré à ssg ou téléchargé au moment du build), ou la configuration ne le désigne pas. La liste du thème explique comment l'y ajouter ; les réglages fonctionnent normalement.",
  },
};
