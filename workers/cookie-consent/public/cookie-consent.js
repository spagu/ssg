/*
 * ssg cookie consent — GDPR / ePrivacy / UK PECR compliant, dependency-free.
 *
 * Design points that make it compliant rather than decorative:
 *   - PRIOR consent: a non-necessary <script type="text/plain"
 *     data-consent-category="analytics"> does not run until its category is
 *     granted. Nothing non-essential fires before a choice.
 *   - Reject is as easy as accept: "Reject all" sits beside "Accept all".
 *   - No pre-ticked boxes: non-necessary toggles start off.
 *   - Withdrawable: any [data-cookie-settings] element (or ssgConsent.open())
 *     reopens the dialog to change or revoke.
 *   - Expiry + versioning: consent is re-asked after expiryDays or when the
 *     policy version changes.
 *   - Google Consent Mode v2 signals and a `ssg:consent` event are emitted so
 *     tag managers react to the choice.
 *
 * Configuration is an inline JSON tag the page renders:
 *   <script id="ssg-consent-config" type="application/json">{...}</script>
 * See the worker README for the full schema.
 */
(function () {
  "use strict";

  var COOKIE = "ssg_consent";

  var DEFAULTS = {
    version: "1",
    policyUrl: "/cookie-policy/",
    position: "bottom", // "bottom" (default) | "top" | "center"
    geoMode: "always", // "always" | "edge" (calls /api/consent/geo)
    geoEndpoint: "/api/consent/geo",
    logEndpoint: "", // optional; POST a proof-of-consent record
    expiryDays: 180,
    defaultLang: "en",
    categories: [
      { id: "necessary", required: true },
      { id: "analytics" },
      { id: "marketing" },
    ],
    i18n: {},
  };

  var STRINGS = {
    en: {
      title: "We value your privacy",
      body: "We use cookies to run this site and, with your consent, to measure traffic and improve it. You can accept, reject, or choose per category.",
      acceptAll: "Accept all", rejectAll: "Reject all", save: "Save choices",
      manage: "Manage cookies", policy: "Cookie policy", close: "Close",
      necessary: "Strictly necessary", necessary_d: "Required for the site to work. Always on.",
      analytics: "Analytics", analytics_d: "Helps us understand how the site is used.",
      marketing: "Marketing", marketing_d: "Used to personalise ads and measure campaigns.",
      preferences: "Preferences", preferences_d: "Remembers choices like language or region.",
    },
    pl: {
      title: "Szanujemy Twoją prywatność",
      body: "Używamy plików cookie do działania strony i — za Twoją zgodą — do pomiaru ruchu i ulepszeń. Możesz zaakceptować, odrzucić lub wybrać per kategoria.",
      acceptAll: "Akceptuj wszystkie", rejectAll: "Odrzuć wszystkie", save: "Zapisz wybór",
      manage: "Ustawienia cookie", policy: "Polityka cookie", close: "Zamknij",
      necessary: "Niezbędne", necessary_d: "Wymagane do działania strony. Zawsze włączone.",
      analytics: "Analityczne", analytics_d: "Pomagają zrozumieć, jak korzystasz ze strony.",
      marketing: "Marketingowe", marketing_d: "Personalizacja reklam i pomiar kampanii.",
      preferences: "Preferencje", preferences_d: "Zapamiętują wybory, np. język lub region.",
    },
    de: {
      title: "Ihre Privatsphäre ist uns wichtig",
      body: "Wir verwenden Cookies für den Betrieb der Website und – mit Ihrer Einwilligung – zur Messung und Verbesserung. Sie können akzeptieren, ablehnen oder je Kategorie wählen.",
      acceptAll: "Alle akzeptieren", rejectAll: "Alle ablehnen", save: "Auswahl speichern",
      manage: "Cookie-Einstellungen", policy: "Cookie-Richtlinie", close: "Schließen",
      necessary: "Unbedingt erforderlich", necessary_d: "Für den Betrieb nötig. Immer aktiv.",
      analytics: "Statistik", analytics_d: "Hilft zu verstehen, wie die Website genutzt wird.",
      marketing: "Marketing", marketing_d: "Personalisierte Werbung und Kampagnenmessung.",
      preferences: "Präferenzen", preferences_d: "Merkt sich Optionen wie Sprache oder Region.",
    },
    fr: {
      title: "Nous respectons votre vie privée",
      body: "Nous utilisons des cookies pour faire fonctionner le site et, avec votre consentement, pour mesurer l'audience et l'améliorer. Vous pouvez accepter, refuser ou choisir par catégorie.",
      acceptAll: "Tout accepter", rejectAll: "Tout refuser", save: "Enregistrer",
      manage: "Gérer les cookies", policy: "Politique cookies", close: "Fermer",
      necessary: "Strictement nécessaires", necessary_d: "Requis au fonctionnement. Toujours actifs.",
      analytics: "Mesure d'audience", analytics_d: "Aide à comprendre l'usage du site.",
      marketing: "Marketing", marketing_d: "Personnalisation des publicités et mesure.",
      preferences: "Préférences", preferences_d: "Mémorise des choix comme la langue.",
    },
    hi: {
      title: "हम आपकी निजता का सम्मान करते हैं",
      body: "हम इस साइट को चलाने के लिए कुकीज़ का उपयोग करते हैं और, आपकी सहमति से, ट्रैफ़िक मापने और साइट को बेहतर बनाने के लिए भी। आप सभी स्वीकार कर सकते हैं, अस्वीकार कर सकते हैं, या हर श्रेणी के लिए अलग से चुन सकते हैं।",
      acceptAll: "सभी स्वीकार करें", rejectAll: "सभी अस्वीकार करें", save: "चुनाव सहेजें",
      manage: "कुकी सेटिंग्स", policy: "कुकी नीति", close: "बंद करें",
      necessary: "अत्यंत आवश्यक", necessary_d: "साइट के काम करने के लिए ज़रूरी। हमेशा चालू।",
      analytics: "एनालिटिक्स", analytics_d: "यह समझने में मदद करती हैं कि साइट का उपयोग कैसे होता है।",
      marketing: "मार्केटिंग", marketing_d: "विज्ञापनों को वैयक्तिकृत करने और अभियानों को मापने के लिए।",
      preferences: "प्राथमिकताएँ", preferences_d: "भाषा या क्षेत्र जैसे विकल्प याद रखती हैं।",
    },
  };

  // ── config + storage ─────────────────────────────────────────────────────

  function readConfig() {
    var el = document.getElementById("ssg-consent-config");
    var cfg = Object.assign({}, DEFAULTS);
    if (el) {
      try {
        Object.assign(cfg, JSON.parse(el.textContent || "{}"));
      } catch (e) {
        console.warn("ssg-consent: invalid config JSON", e);
      }
    }
    return cfg;
  }

  function lang(cfg) {
    var html = (document.documentElement.getAttribute("lang") || "").slice(0, 2).toLowerCase();
    var pick = STRINGS[html] ? html : cfg.defaultLang;
    var base = STRINGS[pick] || STRINGS.en;
    return Object.assign({}, base, (cfg.i18n && cfg.i18n[pick]) || {});
  }

  function getStored() {
    var m = document.cookie.match(/(?:^|;\s*)ssg_consent=([^;]+)/);
    if (!m) return null;
    try {
      return JSON.parse(decodeURIComponent(m[1]));
    } catch (e) {
      return null;
    }
  }

  function setStored(cfg, granted) {
    var value = { v: cfg.version, t: Date.now(), c: granted };
    var maxAge = cfg.expiryDays * 24 * 60 * 60;
    document.cookie =
      COOKIE + "=" + encodeURIComponent(JSON.stringify(value)) +
      ";path=/;max-age=" + maxAge + ";samesite=lax" +
      (location.protocol === "https:" ? ";secure" : "");
  }

  function storedIsValid(cfg, stored) {
    if (!stored || stored.v !== cfg.version) return false;
    var ageDays = (Date.now() - (stored.t || 0)) / (24 * 60 * 60 * 1000);
    return ageDays < cfg.expiryDays;
  }

  // ── effects: gate scripts, signal tag managers, log ──────────────────────

  function activateScripts(granted) {
    var nodes = document.querySelectorAll('script[type="text/plain"][data-consent-category]');
    nodes.forEach(function (old) {
      if (granted.indexOf(old.getAttribute("data-consent-category")) === -1) return;
      var s = document.createElement("script");
      for (var i = 0; i < old.attributes.length; i++) {
        var a = old.attributes[i];
        if (a.name === "type") continue;
        s.setAttribute(a.name, a.value);
      }
      s.text = old.text;
      old.parentNode.replaceChild(s, old);
    });
  }

  function signal(granted) {
    var on = function (cat) { return granted.indexOf(cat) !== -1 ? "granted" : "denied"; };
    window.dataLayer = window.dataLayer || [];
    if (typeof window.gtag === "function") {
      window.gtag("consent", "update", {
        analytics_storage: on("analytics"),
        ad_storage: on("marketing"),
        ad_user_data: on("marketing"),
        ad_personalization: on("marketing"),
        personalization_storage: on("preferences"),
      });
    }
    window.dataLayer.push({ event: "ssg_consent_update", consent: granted });
    document.dispatchEvent(new CustomEvent("ssg:consent", { detail: { granted: granted } }));
  }

  function logConsent(cfg, granted) {
    if (!cfg.logEndpoint) return;
    try {
      fetch(cfg.logEndpoint, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ categories: granted, version: cfg.version }),
        keepalive: true,
      }).catch(function () {});
    } catch (e) { /* best-effort; never blocks the choice */ }
  }

  // apply records an actual CHOICE: persist it, un-gate scripts, signal the tag
  // manager, and write the audit entry. Called only from the banner buttons /
  // Escape — never on a plain page load.
  function apply(cfg, granted) {
    setStored(cfg, granted);
    activateScripts(granted);
    signal(granted);
    logConsent(cfg, granted);
  }

  // reapply re-establishes an already-stored choice on each page load: activate
  // the gated scripts and re-signal Consent Mode (both are per-page), but do NOT
  // re-store (which would slide the expiry to "last visit") and do NOT log (which
  // would append an audit entry on every single pageview, not per consent event).
  function reapply(cfg, granted) {
    activateScripts(granted);
    signal(granted);
  }

  // ── UI ───────────────────────────────────────────────────────────────────

  function categoryIds(cfg) {
    return cfg.categories.map(function (c) { return c.id; });
  }

  function allGranted(cfg) { return categoryIds(cfg); }

  function onlyNecessary(cfg) {
    return cfg.categories.filter(function (c) { return c.required; }).map(function (c) { return c.id; });
  }

  function buildDialog(cfg, t, onDone) {
    var wrap = document.createElement("div");
    var pos = cfg.position === "top" || cfg.position === "center" ? cfg.position : "bottom";
    wrap.className = "ssg-cc ssg-cc--" + pos;
    wrap.setAttribute("role", "dialog");
    wrap.setAttribute("aria-modal", "true");
    wrap.setAttribute("aria-labelledby", "ssg-cc-title");

    var rows = cfg.categories.map(function (c) {
      var label = t[c.id] || c.id;
      var desc = t[c.id + "_d"] || "";
      var checked = c.required ? "checked disabled" : "";
      return (
        '<label class="ssg-cc-cat">' +
        '<input type="checkbox" data-cat="' + c.id + '" ' + checked + ">" +
        "<span><strong>" + label + "</strong><small>" + desc + "</small></span>" +
        "</label>"
      );
    }).join("");

    wrap.innerHTML =
      '<div class="ssg-cc-box">' +
      '<h2 id="ssg-cc-title">' + t.title + "</h2>" +
      "<p>" + t.body + ' <a href="' + cfg.policyUrl + '">' + t.policy + "</a>.</p>" +
      '<div class="ssg-cc-cats">' + rows + "</div>" +
      '<div class="ssg-cc-actions">' +
      '<button type="button" class="ssg-cc-btn ssg-cc-secondary" data-act="reject">' + t.rejectAll + "</button>" +
      '<button type="button" class="ssg-cc-btn ssg-cc-secondary" data-act="save">' + t.save + "</button>" +
      '<button type="button" class="ssg-cc-btn ssg-cc-primary" data-act="accept">' + t.acceptAll + "</button>" +
      "</div></div>";

    wrap.addEventListener("click", function (e) {
      var act = e.target && e.target.getAttribute && e.target.getAttribute("data-act");
      if (!act) return;
      var granted;
      if (act === "accept") granted = allGranted(cfg);
      else if (act === "reject") granted = onlyNecessary(cfg);
      else granted = readToggles(wrap, cfg);
      apply(cfg, granted);
      close(wrap);
      if (onDone) onDone(granted);
    });
    // One document-level handler per open dialog, removed again by close():
    // Escape and the Tab focus trap both have to see keys pressed while focus
    // is anywhere, including on the page behind after a stray click.
    onKey = function (e) {
      if (e.key === "Escape") {
        // Escape saves only necessary — never a silent "accept all".
        apply(cfg, onlyNecessary(cfg));
        close(wrap);
      } else if (e.key === "Tab") {
        trapTab(wrap, e);
      }
    };
    document.addEventListener("keydown", onKey);
    return wrap;
  }

  // ── focus management (ARIA APG modal dialog pattern) ─────────────────────
  //
  // role="dialog" + aria-modal="true" promises assistive technology that the
  // page behind is inert. Keyboard focus has to keep that promise too: Tab and
  // Shift+Tab cycle inside the dialog, and closing it hands focus back to
  // whatever opened it, so a keyboard user is not dropped at the top of the page.

  var FOCUSABLE =
    'a[href], area[href], button, input, select, textarea, iframe, ' +
    '[tabindex]:not([tabindex="-1"]), [contenteditable="true"]';

  var onKey = null; // the open dialog's keydown handler, removed on close
  var returnFocus = null; // the element focused before the dialog opened

  // focusables lists the dialog's tabbable elements in DOM order, skipping
  // disabled controls (the always-on "necessary" toggle), elements opted out
  // with tabindex="-1", and anything not rendered.
  function focusables(wrap) {
    return Array.prototype.filter.call(wrap.querySelectorAll(FOCUSABLE), function (el) {
      if (el.disabled || el.getAttribute("tabindex") === "-1") return false;
      return !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
    });
  }

  // trapTab wraps Tab from the last element to the first and Shift+Tab from
  // the first to the last. Focus that has escaped the dialog (a click on the
  // page behind) is pulled back in on the next Tab.
  function trapTab(wrap, e) {
    var items = focusables(wrap);
    if (!items.length) { e.preventDefault(); return; }
    var first = items[0];
    var last = items[items.length - 1];
    var active = document.activeElement;
    var inside = wrap.contains(active);
    if (e.shiftKey && (!inside || active === first)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (!inside || active === last)) {
      e.preventDefault();
      first.focus();
    }
  }

  // restoreFocus returns focus to the opener, but only if it is still on the
  // page and is something real — focusing <body> or a detached node does
  // nothing useful and can scroll the page.
  function restoreFocus() {
    var el = returnFocus;
    returnFocus = null;
    if (!el || el === document.body || el === document.documentElement) return;
    if (!document.documentElement.contains(el) || typeof el.focus !== "function") return;
    el.focus();
  }

  function readToggles(wrap, cfg) {
    var granted = onlyNecessary(cfg);
    wrap.querySelectorAll('input[data-cat]:checked').forEach(function (i) {
      var id = i.getAttribute("data-cat");
      if (granted.indexOf(id) === -1) granted.push(id);
    });
    return granted;
  }

  // open shows the dialog. `opener` is the element that asked for it (a
  // [data-cookie-settings] link); without one, whatever had focus is used.
  // Passing it explicitly matters on Safari, which does not focus a link on click.
  function open(cfg, t, opener) {
    if (document.querySelector(".ssg-cc")) return;
    returnFocus = opener || document.activeElement;
    var stored = getStored();
    var dlg = buildDialog(cfg, t);
    document.body.appendChild(dlg);
    // Pre-check previously granted categories when reopening.
    if (stored && stored.c) {
      dlg.querySelectorAll('input[data-cat]').forEach(function (i) {
        if (stored.c.indexOf(i.getAttribute("data-cat")) !== -1) i.checked = true;
      });
    }
    var first = dlg.querySelector("button");
    if (first) first.focus();
  }

  function close(wrap) {
    if (onKey) document.removeEventListener("keydown", onKey);
    onKey = null;
    if (wrap && wrap.parentNode) wrap.parentNode.removeChild(wrap);
    restoreFocus();
  }

  // ── boot ───────────────────────────────────────────────────────────────

  function needBanner(cfg, done) {
    if (storedIsValid(cfg, getStored())) return done(false);
    if (cfg.geoMode !== "edge") return done(true);
    fetch(cfg.geoEndpoint, { headers: { accept: "application/json" } })
      .then(function (r) { return r.ok ? r.json() : { required: true }; })
      .then(function (d) { done(!!d.required); })
      .catch(function () { done(true); }); // fail safe: ask rather than skip
  }

  function boot() {
    var cfg = readConfig();
    var t = lang(cfg);

    // Re-apply an already-stored choice on every load (activate gated scripts,
    // re-signal), so consent persists across pages without re-asking — but
    // without re-storing or re-logging it (see reapply).
    var stored = getStored();
    if (storedIsValid(cfg, stored)) reapply(cfg, stored.c || onlyNecessary(cfg));

    // Public API + a "manage cookies" hook for any element.
    window.ssgConsent = {
      open: function () { open(cfg, t); },
      granted: function (cat) {
        var s = getStored();
        return !!(s && s.c && s.c.indexOf(cat) !== -1);
      },
      categories: categoryIds(cfg),
    };
    document.addEventListener("click", function (e) {
      var el = e.target && e.target.closest && e.target.closest("[data-cookie-settings]");
      if (el) { e.preventDefault(); open(cfg, t, el); }
    });

    needBanner(cfg, function (show) { if (show) open(cfg, t); });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
