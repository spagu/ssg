// The summary's sentences in the five panel languages. Fixed sentences: the
// only value written into one is the branch name, which validBranch has
// limited to letters, digits and ._/- (no person's data ever).

import type { Text } from "./summary-kit";

export const LABELS: Record<"daysSinceContentChange" | "contentFolders", Text> = {
  daysSinceContentChange: {
    en: "Days since content last changed",
    pl: "Dni od ostatniej zmiany treści",
    hi: "सामग्री में आख़िरी बदलाव के बाद से दिन",
    de: "Tage seit der letzten Inhaltsänderung",
    fr: "Jours depuis la dernière modification du contenu",
  },
  contentFolders: {
    en: "Content folders",
    pl: "Katalogi treści",
    hi: "सामग्री फ़ोल्डर",
    de: "Inhaltsordner",
    fr: "Dossiers de contenu",
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
    en: "The repository is not configured: EDITOR_GIT_REPO or the EDITOR_GIT_TOKEN secret is missing or malformed. The health route says which.",
    pl: "Repozytorium nie jest skonfigurowane: brakuje EDITOR_GIT_REPO albo sekretu EDITOR_GIT_TOKEN lub mają zły format. Trasa health mówi, której.",
    hi: "रिपॉज़िटरी कॉन्फ़िगर नहीं है: EDITOR_GIT_REPO या सीक्रेट EDITOR_GIT_TOKEN नहीं है या उसका रूप गलत है। health रूट बताता है कि कौन-सा।",
    de: "Das Repository ist nicht konfiguriert: EDITOR_GIT_REPO oder das Secret EDITOR_GIT_TOKEN fehlt oder ist fehlerhaft. Die Health-Route sagt, welches.",
    fr: "Le dépôt n'est pas configuré : EDITOR_GIT_REPO ou le secret EDITOR_GIT_TOKEN manque ou est mal formé. La route health indique lequel.",
  },
  repo_token_rejected: {
    en: "The repository host refuses the token EDITOR_GIT_TOKEN: it has expired, was revoked, or lacks access to this repository. Nothing can be read or published.",
    pl: "Dostawca repozytorium odrzuca token EDITOR_GIT_TOKEN: wygasł, został unieważniony albo nie ma dostępu do tego repozytorium. Niczego nie da się odczytać ani opublikować.",
    hi: "रिपॉज़िटरी होस्ट टोकन EDITOR_GIT_TOKEN को अस्वीकार करता है: उसकी अवधि खत्म हो गई है, उसे रद्द किया गया है, या उसके पास इस रिपॉज़िटरी की पहुँच नहीं है। कुछ भी पढ़ा या प्रकाशित नहीं किया जा सकता।",
    de: "Der Repository-Anbieter lehnt das Token EDITOR_GIT_TOKEN ab: Es ist abgelaufen, wurde widerrufen oder hat keinen Zugriff auf dieses Repository. Nichts kann gelesen oder veröffentlicht werden.",
    fr: "L'hébergeur du dépôt refuse le jeton EDITOR_GIT_TOKEN : il a expiré, a été révoqué ou n'a pas accès à ce dépôt. Rien ne peut être lu ni publié.",
  },
  repo_not_found: {
    en: "The repository or the branch in EDITOR_GIT_REPO / EDITOR_GIT_BRANCH does not exist, or the token cannot see it.",
    pl: "Repozytorium albo gałąź z EDITOR_GIT_REPO / EDITOR_GIT_BRANCH nie istnieje albo token jej nie widzi.",
    hi: "EDITOR_GIT_REPO / EDITOR_GIT_BRANCH में दी गई रिपॉज़िटरी या ब्रांच मौजूद नहीं है, या टोकन उसे देख नहीं सकता।",
    de: "Das Repository oder der Branch aus EDITOR_GIT_REPO / EDITOR_GIT_BRANCH existiert nicht, oder das Token sieht ihn nicht.",
    fr: "Le dépôt ou la branche indiqués dans EDITOR_GIT_REPO / EDITOR_GIT_BRANCH n'existent pas, ou le jeton ne les voit pas.",
  },
  repo_unreachable: {
    en: "The repository host did not answer as expected (unavailable or rate-limited). Try again later.",
    pl: "Dostawca repozytorium nie odpowiedział zgodnie z oczekiwaniem (niedostępny albo przekroczony limit zapytań). Spróbuj ponownie później.",
    hi: "रिपॉज़िटरी होस्ट ने अपेक्षा के अनुसार जवाब नहीं दिया (उपलब्ध नहीं है या अनुरोधों की सीमा पूरी हो गई)। बाद में फिर कोशिश करें।",
    de: "Der Repository-Anbieter hat nicht wie erwartet geantwortet (nicht erreichbar oder Anfragelimit erreicht). Versuchen Sie es später erneut.",
    fr: "L'hébergeur du dépôt n'a pas répondu comme prévu (indisponible ou limite de requêtes atteinte). Réessayez plus tard.",
  },
  site_config_unreadable: {
    en: "The site's configuration file on the branch cannot be read by the editor. Set EDITOR_CONTENT_DIRS and EDITOR_MEDIA_DIR in wrangler.toml; until then, nothing can be edited.",
    pl: "Edytor nie potrafi odczytać pliku konfiguracji strony z gałęzi. Ustaw EDITOR_CONTENT_DIRS i EDITOR_MEDIA_DIR w wrangler.toml; do tego czasu niczego nie da się edytować.",
    hi: "संपादक ब्रांच पर साइट की कॉन्फ़िगरेशन फ़ाइल नहीं पढ़ सकता। wrangler.toml में EDITOR_CONTENT_DIRS और EDITOR_MEDIA_DIR सेट करें; तब तक कुछ भी संपादित नहीं किया जा सकता।",
    de: "Der Editor kann die Konfigurationsdatei der Website auf dem Branch nicht lesen. Setzen Sie EDITOR_CONTENT_DIRS und EDITOR_MEDIA_DIR in wrangler.toml; bis dahin lässt sich nichts bearbeiten.",
    fr: "L'éditeur ne peut pas lire le fichier de configuration du site sur la branche. Définissez EDITOR_CONTENT_DIRS et EDITOR_MEDIA_DIR dans wrangler.toml ; d'ici là, rien ne peut être modifié.",
  },
  site_config_overridden: {
    en: "The site's configuration file cannot be read by the editor, so the content folders come from EDITOR_CONTENT_DIRS; languages and shortcodes are unknown.",
    pl: "Edytor nie potrafi odczytać pliku konfiguracji strony, więc katalogi treści pochodzą z EDITOR_CONTENT_DIRS; języki i shortcode'y są nieznane.",
    hi: "संपादक साइट की कॉन्फ़िगरेशन फ़ाइल नहीं पढ़ सकता, इसलिए सामग्री फ़ोल्डर EDITOR_CONTENT_DIRS से लिए गए हैं; भाषाएँ और शॉर्टकोड अज्ञात हैं।",
    de: "Der Editor kann die Konfigurationsdatei der Website nicht lesen, daher kommen die Inhaltsordner aus EDITOR_CONTENT_DIRS; Sprachen und Shortcodes sind unbekannt.",
    fr: "L'éditeur ne peut pas lire le fichier de configuration du site : les dossiers de contenu viennent donc de EDITOR_CONTENT_DIRS ; les langues et les shortcodes sont inconnus.",
  },
  no_content_dirs: {
    en: "The site's configuration names no content folder (source or content_sources), so there is nothing to edit. Set EDITOR_CONTENT_DIRS in wrangler.toml.",
    pl: "Konfiguracja strony nie wskazuje żadnego katalogu treści (source ani content_sources), więc nie ma czego edytować. Ustaw EDITOR_CONTENT_DIRS w wrangler.toml.",
    hi: "साइट का कॉन्फ़िगरेशन कोई सामग्री फ़ोल्डर (source या content_sources) नहीं बताता, इसलिए संपादित करने को कुछ नहीं है। wrangler.toml में EDITOR_CONTENT_DIRS सेट करें।",
    de: "Die Konfiguration der Website nennt keinen Inhaltsordner (source oder content_sources), es gibt also nichts zu bearbeiten. Setzen Sie EDITOR_CONTENT_DIRS in wrangler.toml.",
    fr: "La configuration du site ne désigne aucun dossier de contenu (source ou content_sources) : il n'y a rien à modifier. Définissez EDITOR_CONTENT_DIRS dans wrangler.toml.",
  },
  bad_override: {
    en: "EDITOR_CONTENT_DIRS or EDITOR_MEDIA_DIR holds something that is not a folder inside the repository (one folder only for the media).",
    pl: "EDITOR_CONTENT_DIRS albo EDITOR_MEDIA_DIR zawiera coś, co nie jest katalogiem w repozytorium (dla mediów tylko jeden katalog).",
    hi: "EDITOR_CONTENT_DIRS या EDITOR_MEDIA_DIR में कुछ ऐसा है जो रिपॉज़िटरी के अंदर का फ़ोल्डर नहीं है (मीडिया के लिए केवल एक फ़ोल्डर)।",
    de: "EDITOR_CONTENT_DIRS oder EDITOR_MEDIA_DIR enthält etwas, das kein Ordner im Repository ist (für Medien nur ein Ordner).",
    fr: "EDITOR_CONTENT_DIRS ou EDITOR_MEDIA_DIR contient autre chose qu'un dossier du dépôt (un seul dossier pour les médias).",
  },
  site_config_partial: {
    en: "Some settings in the site's configuration were left out because they are not folders inside the repository or are set at build time.",
    pl: "Część ustawień z konfiguracji strony pominięto, bo nie są katalogami w repozytorium albo są ustawiane dopiero przy buildzie.",
    hi: "साइट के कॉन्फ़िगरेशन की कुछ सेटिंग छोड़ दी गईं, क्योंकि वे रिपॉज़िटरी के अंदर के फ़ोल्डर नहीं हैं या बिल्ड के समय सेट होती हैं।",
    de: "Einige Einstellungen der Website-Konfiguration wurden ausgelassen, weil sie keine Ordner im Repository sind oder erst beim Build gesetzt werden.",
    fr: "Certains réglages de la configuration du site ont été ignorés : ce ne sont pas des dossiers du dépôt, ou ils sont fixés au moment du build.",
  },
};
