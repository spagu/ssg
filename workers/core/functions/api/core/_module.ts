// The module manifest (GO-143) as code. Pages Functions cannot read files at
// runtime and the site build copies only functions/, so the manifest route
// imports this copy of module.json (the template root's). Generated: edit
// module.json, then run UPDATE_MODULE_TWINS=1 node --test 'workers/*.test.ts'
// in the ssg repository, or copy the JSON here by hand — the two must match.
// A leading underscore keeps this file out of the Pages route table.
export const MODULE = {
  "id": "core",
  "contract": 1,
  "version": "1.0.0",
  "kind": "pages-function",
  "title": {
    "en": "Admin core",
    "pl": "Rdzeń administracji",
    "hi": "एडमिन कोर",
    "de": "Verwaltungskern",
    "fr": "Noyau d'administration"
  },
  "description": {
    "en": "Knows which modules the site runs: lists them for the shared navigation, checks which of them answer, and gathers their numbers and what needs attention on the admin home page. It links to each module's panel; every module still checks access itself.",
    "pl": "Wie, jakie moduły ma strona: podaje ich listę wspólnej nawigacji, sprawdza, które odpowiadają, i zbiera ich liczby oraz sprawy do załatwienia na stronie głównej administracji. Linkuje do panelu każdego modułu; każdy moduł nadal sam sprawdza dostęp.",
    "hi": "जानता है कि साइट पर कौन-से मॉड्यूल चलते हैं: साझा नेविगेशन के लिए उनकी सूची देता है, जाँचता है कि कौन-से जवाब देते हैं, और एडमिन होम पेज पर उनकी संख्याएँ और ध्यान माँगने वाली बातें इकट्ठा करता है। यह हर मॉड्यूल के पैनल से लिंक करता है; हर मॉड्यूल पहुँच की जाँच अब भी ख़ुद करता है।",
    "de": "Weiß, welche Module die Website betreibt: listet sie für die gemeinsame Navigation auf, prüft, welche antworten, und sammelt auf der Verwaltungsstartseite ihre Zahlen und was Aufmerksamkeit braucht. Es verlinkt auf die Verwaltung jedes Moduls; jedes Modul prüft den Zugriff weiterhin selbst.",
    "fr": "Sait quels modules le site utilise : les liste pour la navigation commune, vérifie lesquels répondent et rassemble sur la page d'accueil de l'administration leurs chiffres et ce qui demande une action. Il renvoie vers le panneau de chaque module ; chaque module continue de vérifier l'accès lui-même."
  },
  "panel": "/core/",
  "nav": [],
  "permissions": [
    {
      "name": "core:manage",
      "description": {
        "en": "Change how modules appear in the shared navigation and on the admin home page: order, visibility and labels. Hiding a module there does not switch it off.",
        "pl": "Zmieniać, jak moduły wyglądają we wspólnej nawigacji i na stronie głównej administracji: kolejność, widoczność i etykiety. Ukrycie modułu nie wyłącza go.",
        "hi": "साझा नेविगेशन और एडमिन होम पेज पर मॉड्यूल कैसे दिखें, यह बदलना: क्रम, दृश्यता और लेबल। वहाँ किसी मॉड्यूल को छिपाने से वह बंद नहीं होता।",
        "de": "Ändern, wie Module in der gemeinsamen Navigation und auf der Verwaltungsstartseite erscheinen: Reihenfolge, Sichtbarkeit und Beschriftungen. Ein dort ausgeblendetes Modul ist nicht abgeschaltet.",
        "fr": "Modifier l'affichage des modules dans la navigation commune et sur la page d'accueil de l'administration : ordre, visibilité et libellés. Masquer un module ne le désactive pas."
      }
    }
  ],
  "endpoints": {
    "manifest": "/api/core/module"
  },
  "disable": {
    "en": "Remove workers/core from the workers: list in .ssg.yaml, rebuild and deploy. The other modules keep working on their own; the shared navigation simply stops appearing.",
    "pl": "Usuń workers/core z listy workers: w .ssg.yaml, zbuduj stronę ponownie i wdróż. Pozostałe moduły działają dalej samodzielnie; wspólna nawigacja po prostu przestaje się pojawiać.",
    "hi": ".ssg.yaml की workers: सूची से workers/core हटाएँ, फिर से बिल्ड करें और डिप्लॉय करें। बाक़ी मॉड्यूल अपने-आप चलते रहते हैं; साझा नेविगेशन बस दिखना बंद हो जाता है।",
    "de": "Entfernen Sie workers/core aus der Liste workers: in .ssg.yaml, bauen Sie neu und stellen Sie bereit. Die anderen Module arbeiten allein weiter; die gemeinsame Navigation erscheint einfach nicht mehr.",
    "fr": "Retirez workers/core de la liste workers: dans .ssg.yaml, reconstruisez et déployez. Les autres modules continuent de fonctionner seuls ; la navigation commune n'apparaît simplement plus."
  }
};
