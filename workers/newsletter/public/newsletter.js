/*
 * ssg newsletter form — progressive enhancement, dependency-free.
 *
 * The form in newsletter-form.html works with no script at all: the browser
 * posts it, the worker answers 303 to the thank-you page. This file only makes
 * it nicer where JavaScript runs: it submits with fetch, keeps the visitor on
 * the page, and reports the result in the form's aria-live status region in the
 * page's language.
 *
 * It enhances every <form data-ssg-newsletter>. Optional config, as for the
 * other SSG widgets:
 *   <script id="ssg-newsletter-config" type="application/json">
 *     { "defaultLang": "en", "i18n": { "en": { "ok": "Welcome aboard!" } } }
 *   </script>
 */
(function () {
  "use strict";

  var STRINGS = {
    en: {
      sending: "Subscribing…",
      ok: "Thank you — you are subscribed.",
      okConfirm: "Almost done: check your inbox and confirm with the link we sent you.",
      invalid_email: "Please enter a valid email address.",
      consent_required: "Please tick the consent box to subscribe.",
      captcha_failed: "The anti-spam check did not pass. Please try again.",
      rate_limited: "Too many attempts. Please wait a minute and try again.",
      error: "Something went wrong. Please try again later.",
    },
    pl: {
      sending: "Zapisuję…",
      ok: "Dziękujemy — zapisano Cię do newslettera.",
      okConfirm: "Jeszcze chwila: sprawdź skrzynkę i potwierdź zapis linkiem, który wysłaliśmy.",
      invalid_email: "Wpisz poprawny adres e-mail.",
      consent_required: "Zaznacz zgodę, aby się zapisać.",
      captcha_failed: "Weryfikacja antyspamowa nie powiodła się. Spróbuj ponownie.",
      rate_limited: "Zbyt wiele prób. Odczekaj minutę i spróbuj ponownie.",
      error: "Coś poszło nie tak. Spróbuj ponownie później.",
    },
    hi: {
      sending: "सदस्यता ली जा रही है…",
      ok: "धन्यवाद — आपने सदस्यता ले ली है।",
      okConfirm: "बस एक कदम बाकी: अपना इनबॉक्स देखें और हमारे भेजे लिंक से पुष्टि करें।",
      invalid_email: "कृपया एक मान्य ईमेल पता दर्ज करें।",
      consent_required: "सदस्यता लेने के लिए कृपया सहमति वाले बॉक्स पर निशान लगाएँ।",
      captcha_failed: "स्पैम-रोधी जाँच सफल नहीं हुई। कृपया फिर से प्रयास करें।",
      rate_limited: "बहुत अधिक प्रयास। कृपया एक मिनट रुककर फिर से प्रयास करें।",
      error: "कुछ गड़बड़ हो गई। कृपया बाद में फिर से प्रयास करें।",
    },
    de: {
      sending: "Wird angemeldet…",
      ok: "Vielen Dank — Sie sind angemeldet.",
      okConfirm: "Fast geschafft: Bitte bestätigen Sie die Anmeldung über den Link in unserer E-Mail.",
      invalid_email: "Bitte geben Sie eine gültige E-Mail-Adresse ein.",
      consent_required: "Bitte setzen Sie das Häkchen bei der Einwilligung.",
      captcha_failed: "Die Spam-Prüfung ist fehlgeschlagen. Bitte versuchen Sie es erneut.",
      rate_limited: "Zu viele Versuche. Bitte warten Sie eine Minute.",
      error: "Etwas ist schiefgelaufen. Bitte versuchen Sie es später erneut.",
    },
    fr: {
      sending: "Inscription en cours…",
      ok: "Merci — vous êtes inscrit.",
      okConfirm: "Presque fini : confirmez votre inscription avec le lien que nous venons d'envoyer.",
      invalid_email: "Saisissez une adresse e-mail valide.",
      consent_required: "Cochez la case de consentement pour vous inscrire.",
      captcha_failed: "La vérification anti-spam a échoué. Réessayez.",
      rate_limited: "Trop de tentatives. Patientez une minute avant de réessayer.",
      error: "Une erreur est survenue. Réessayez plus tard.",
    },
  };

  function readConfig() {
    var el = document.getElementById("ssg-newsletter-config");
    if (!el) return {};
    try {
      return JSON.parse(el.textContent || "{}");
    } catch (e) {
      console.warn("ssg-newsletter: invalid config JSON", e);
      return {};
    }
  }

  // strings follows <html lang> (or the form's own lang attribute), falling back
  // to defaultLang, then English; i18n.<lang> overrides single keys.
  function strings(cfg, form) {
    var tag = (form.getAttribute("lang") || document.documentElement.getAttribute("lang") || "").slice(0, 2).toLowerCase();
    var known = STRINGS[tag] || (cfg.i18n && cfg.i18n[tag]);
    var pick = known ? tag : cfg.defaultLang || "en";
    return Object.assign({}, STRINGS.en, STRINGS[pick] || {}, (cfg.i18n && cfg.i18n[pick]) || {});
  }

  // fillDefaults completes the hidden fields a theme may leave empty, so the
  // stored record says which page and language the sign-up came from.
  function fillDefaults(form) {
    var lang = form.querySelector('input[name="language"]');
    if (lang && !lang.value) lang.value = document.documentElement.getAttribute("lang") || "";
    var page = form.querySelector('input[name="source_page"]');
    if (page && !page.value) page.value = location.pathname;
  }

  // payload turns the form into the JSON the worker expects. consent_text is
  // read from the visible label at submit time, so the stored proof matches the
  // words actually on screen even after an i18n override changed them.
  function payload(form) {
    var data = new FormData(form);
    var label = form.querySelector("[data-nl-consent-text]");
    var tags = [];
    data.getAll("tags").forEach(function (v) {
      String(v).split(",").forEach(function (t) { if (t.trim()) tags.push(t.trim()); });
    });
    return {
      email: data.get("email") || "",
      consent: !!form.querySelector('input[name="consent"]:checked'),
      consent_text: label ? label.textContent.replace(/\s+/g, " ").trim() : data.get("consent_text") || "",
      language: data.get("language") || "",
      source_page: data.get("source_page") || "",
      tags: tags,
      token: data.get("cf-turnstile-response") || "",
    };
  }

  function enhance(form, cfg) {
    var t = strings(cfg, form);
    var status = form.querySelector("[data-nl-status]");
    var button = form.querySelector('button[type="submit"], button:not([type])');
    var say = function (msg) { if (status) status.textContent = msg; };
    fillDefaults(form);

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (form.reportValidity && !form.reportValidity()) return;
      if (button) button.disabled = true;
      say(t.sending);
      fetch(form.action, {
        method: "POST",
        headers: { "content-type": "application/json", accept: "application/json" },
        body: JSON.stringify(payload(form)),
      })
        .then(function (res) {
          return res.json().catch(function () { return {}; }).then(function (body) {
            if (res.ok && body.ok) {
              say(body.confirm ? t.okConfirm : t.ok);
              form.reset();
              fillDefaults(form);
            } else {
              say(res.status === 429 ? t.rate_limited : t[body.error] || t.error);
            }
          });
        })
        .catch(function () { say(t.error); })
        .then(function () {
          if (button) button.disabled = false;
          // A Turnstile token is single-use: get a fresh one for the next try.
          if (window.turnstile && typeof window.turnstile.reset === "function") {
            try { window.turnstile.reset(); } catch (err) { /* no widget rendered */ }
          }
        });
    });
  }

  function boot() {
    var cfg = readConfig();
    document.querySelectorAll("form[data-ssg-newsletter]").forEach(function (f) { enhance(f, cfg); });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
