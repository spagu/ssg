// The module manifest (GO-143) as code. Pages Functions cannot read files at
// runtime and the site build copies only functions/, so the manifest route
// imports this copy of module.json (the template root's). Generated: edit
// module.json, then run UPDATE_MODULE_TWINS=1 node --test 'workers/*.test.ts'
// in the ssg repository, or copy the JSON here by hand — the two must match.
// A leading underscore keeps this file out of the Pages route table.
export const MODULE = {
  "id": "newsletter",
  "contract": 1,
  "version": "1.0.0",
  "kind": "pages-function",
  "title": {
    "en": "Newsletter sign-up",
    "pl": "Zapis do newslettera",
    "hi": "न्यूज़लेटर सब्सक्रिप्शन",
    "de": "Newsletter-Anmeldung",
    "fr": "Inscription à la newsletter"
  },
  "description": {
    "en": "Newsletter or waitlist sign-up: addresses in D1, consent recorded with its wording, one-click unsubscribe, optional double opt-in.",
    "pl": "Zapis do newslettera lub listy oczekujących: adresy w D1, zgoda zapisana wraz z jej treścią, wypisanie jednym kliknięciem, opcjonalne podwójne potwierdzenie.",
    "hi": "न्यूज़लेटर या वेटलिस्ट साइन-अप: पते D1 में, सहमति उसके शब्दों के साथ दर्ज, एक क्लिक में अनसब्सक्राइब, वैकल्पिक डबल ऑप्ट-इन।",
    "de": "Anmeldung zu Newsletter oder Warteliste: Adressen in D1, Einwilligung samt Wortlaut protokolliert, Abmeldung mit einem Klick, optionales Double-Opt-in.",
    "fr": "Inscription à une newsletter ou à une liste d'attente : adresses dans D1, consentement enregistré avec sa formulation, désinscription en un clic, double opt-in facultatif."
  },
  "panel": null,
  "nav": [],
  "permissions": [
    {
      "name": "newsletter:read",
      "description": {
        "en": "View subscribers, counts, campaigns and statistics.",
        "pl": "Przeglądanie subskrybentów, liczników, kampanii i statystyk.",
        "hi": "सब्सक्राइबर, गिनती, कैंपेन और आँकड़े देखना।",
        "de": "Abonnenten, Zählungen, Kampagnen und Statistiken ansehen.",
        "fr": "Consulter les abonnés, les compteurs, les campagnes et les statistiques."
      }
    },
    {
      "name": "newsletter:export",
      "description": {
        "en": "Download subscribers as CSV and export one person's data.",
        "pl": "Pobieranie subskrybentów jako CSV i eksport danych jednej osoby.",
        "hi": "सब्सक्राइबर CSV के रूप में डाउनलोड करना और किसी एक व्यक्ति का डेटा एक्सपोर्ट करना।",
        "de": "Abonnenten als CSV herunterladen und die Daten einer Person exportieren.",
        "fr": "Télécharger les abonnés en CSV et exporter les données d'une personne."
      }
    },
    {
      "name": "newsletter:settings",
      "description": {
        "en": "Maintenance: test the SMTP connection, read the audit trail of admin actions, and run the retention purge now.",
        "pl": "Utrzymanie: test połączenia SMTP, wgląd w dziennik działań administratorów i natychmiastowe czyszczenie danych po okresie przechowywania.",
        "hi": "रखरखाव: SMTP कनेक्शन जाँचना, एडमिन कार्रवाइयों का ऑडिट लॉग देखना, और रखने की अवधि पूरी कर चुके डेटा की सफ़ाई अभी चलाना।",
        "de": "Wartung: die SMTP-Verbindung testen, das Protokoll der Verwaltungsaktionen einsehen und die Bereinigung nach Aufbewahrungsfrist sofort starten.",
        "fr": "Maintenance : tester la connexion SMTP, consulter le journal des actions d'administration et lancer tout de suite la purge selon la durée de conservation."
      }
    }
  ],
  "endpoints": {
    "manifest": "/api/newsletter/module",
    "summary": "/api/newsletter/module/summary"
  },
  "disable": {
    "en": "Remove workers/newsletter from the workers: list in .ssg.yaml, take the sign-up form out of your templates and deploy the site again. Subscribers stay in the D1 database; newsletter-ui reads the same table, so switch it off first if you run it.",
    "pl": "Usuń workers/newsletter z listy workers: w .ssg.yaml, wyjmij formularz zapisu z szablonów i wdróż stronę ponownie. Subskrybenci zostają w bazie D1; newsletter-ui czyta tę samą tabelę, więc jeśli go używasz, wyłącz go najpierw.",
    "hi": "workers/newsletter को .ssg.yaml की workers: सूची से हटाएँ, अपने टेम्पलेट से साइन-अप फ़ॉर्म निकालें और साइट फिर से डिप्लॉय करें। सब्सक्राइबर D1 डेटाबेस में रहते हैं; newsletter-ui उसी टेबल को पढ़ता है, इसलिए अगर आप उसे चलाते हैं तो पहले उसे बंद करें।",
    "de": "Entfernen Sie workers/newsletter aus der workers:-Liste in .ssg.yaml, nehmen Sie das Anmeldeformular aus Ihren Templates und veröffentlichen Sie die Website neu. Die Abonnenten bleiben in der D1-Datenbank; newsletter-ui liest dieselbe Tabelle, schalten Sie es also zuerst ab, falls Sie es nutzen.",
    "fr": "Retirez workers/newsletter de la liste workers: dans .ssg.yaml, enlevez le formulaire d'inscription de vos modèles et redéployez le site. Les abonnés restent dans la base D1 ; newsletter-ui lit la même table, désactivez-le donc d'abord si vous l'utilisez."
  }
};
