/*
 * ssg comments — a dependency-free comments widget.
 *
 * Mount point (the theme renders this on a post):
 *   <div id="ssg-comments" data-url="/blog/my-post/"></div>
 *   <script id="ssg-comments-config" type="application/json">
 *     { "turnstileSiteKey": "0x...", "api": "/api/comments", "order": "newest" }
 *   </script>
 *   <script src="/comments.js" defer></script>
 *
 * User content is always inserted as text (never innerHTML), so a comment
 * cannot inject markup. New comments are held for moderation, so nothing a
 * visitor writes appears until an admin approves it.
 */
(function () {
  "use strict";

  var DEFAULTS = {
    api: "/api/comments",
    order: "newest",
    turnstileSiteKey: "",
    defaultLang: "en",
    i18n: {},
  };

  // UI strings. The active language is <html lang> if we ship it, else
  // cfg.defaultLang, else English; cfg.i18n[lang] overrides any key, so a site
  // can retranslate or add a language without editing this file.
  var STRINGS = {
    en: {
      title: "Comments", empty: "No comments yet. Be the first.",
      formTitle: "Leave a comment",
      name: "Name", email: "Email (optional, for your avatar — never shown)",
      body: "Your comment", submit: "Post comment",
      sending: "Sending…", thanks: "Thanks — your comment is awaiting review.",
      error: "Could not post your comment.", network: "Network error — please try again.",
      closed: "Comments are closed for this post.",
    },
    pl: {
      title: "Komentarze", empty: "Brak komentarzy. Bądź pierwszy.",
      formTitle: "Dodaj komentarz",
      name: "Imię", email: "E-mail (opcjonalnie, do awatara — nigdy nie pokazywany)",
      body: "Twój komentarz", submit: "Opublikuj komentarz",
      sending: "Wysyłanie…", thanks: "Dziękujemy — komentarz czeka na moderację.",
      error: "Nie udało się dodać komentarza.", network: "Błąd sieci — spróbuj ponownie.",
      closed: "Komentarze do tego wpisu są zamknięte.",
    },
    de: {
      title: "Kommentare", empty: "Noch keine Kommentare. Schreiben Sie den ersten.",
      formTitle: "Kommentar hinterlassen",
      name: "Name", email: "E-Mail (optional, für Ihren Avatar — nie angezeigt)",
      body: "Ihr Kommentar", submit: "Kommentar absenden",
      sending: "Senden…", thanks: "Danke — Ihr Kommentar wird geprüft.",
      error: "Kommentar konnte nicht gesendet werden.", network: "Netzwerkfehler — bitte erneut versuchen.",
      closed: "Die Kommentare zu diesem Beitrag sind geschlossen.",
    },
    fr: {
      title: "Commentaires", empty: "Aucun commentaire. Soyez le premier.",
      formTitle: "Laisser un commentaire",
      name: "Nom", email: "E-mail (facultatif, pour votre avatar — jamais affiché)",
      body: "Votre commentaire", submit: "Publier le commentaire",
      sending: "Envoi…", thanks: "Merci — votre commentaire est en attente de validation.",
      error: "Impossible de publier votre commentaire.", network: "Erreur réseau — veuillez réessayer.",
      closed: "Les commentaires sont fermés pour cet article.",
    },
  };

  function strings(cfg) {
    var html = (document.documentElement.getAttribute("lang") || "").slice(0, 2).toLowerCase();
    var pick = STRINGS[html] ? html : cfg.defaultLang;
    var base = STRINGS[pick] || STRINGS.en;
    return Object.assign({}, base, (cfg.i18n && cfg.i18n[pick]) || {});
  }

  function readConfig() {
    var el = document.getElementById("ssg-comments-config");
    var cfg = Object.assign({}, DEFAULTS);
    if (el) {
      try {
        Object.assign(cfg, JSON.parse(el.textContent || "{}"));
      } catch (e) {
        console.warn("ssg-comments: invalid config JSON", e);
      }
    }
    return cfg;
  }

  function el(tag, attrs, text) {
    var node = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) { node.setAttribute(k, attrs[k]); });
    }
    if (text != null) node.textContent = text; // text, never HTML
    return node;
  }

  function formatDate(iso) {
    try {
      return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
    } catch (e) {
      return iso;
    }
  }

  function renderList(root, data, t) {
    var list = el("ol", { class: "ssg-comments-list" });
    if (!data.comments.length) {
      root.appendChild(el("p", { class: "ssg-comments-empty" }, t.empty));
    }
    data.comments.forEach(function (c) {
      var item = el("li", { class: "ssg-comment" });
      var head = el("div", { class: "ssg-comment-head" });
      if (c.avatar_hash) {
        head.appendChild(el("img", {
          class: "ssg-comment-avatar", width: "36", height: "36", alt: "",
          src: "https://www.gravatar.com/avatar/" + c.avatar_hash + "?s=72&d=identicon",
        }));
      }
      head.appendChild(el("span", { class: "ssg-comment-author" }, c.author));
      head.appendChild(el("time", { datetime: c.created_at, class: "ssg-comment-date" }, formatDate(c.created_at)));
      item.appendChild(head);
      item.appendChild(el("div", { class: "ssg-comment-body" }, c.body));
      list.appendChild(item);
    });
    root.appendChild(el("h2", { class: "ssg-comments-title" }, t.title + " (" + data.count + ")"));
    root.appendChild(list);
  }

  function renderForm(root, cfg, url, t, published, onPosted) {
    var form = el("form", { class: "ssg-comments-form", novalidate: "" });
    form.appendChild(el("h3", null, t.formTitle));

    var name = el("input", { type: "text", name: "author", required: "", maxlength: "80", placeholder: t.name, "aria-label": t.name });
    var email = el("input", { type: "email", name: "email", maxlength: "254", placeholder: t.email, "aria-label": t.email });
    var body = el("textarea", { name: "body", required: "", rows: "4", maxlength: "5000", placeholder: t.body, "aria-label": t.body });
    form.appendChild(name);
    form.appendChild(email);
    form.appendChild(body);

    if (cfg.turnstileSiteKey) {
      form.appendChild(el("div", { class: "cf-turnstile", "data-sitekey": cfg.turnstileSiteKey }));
    }

    var status = el("p", { class: "ssg-comments-status", role: "status", "aria-live": "polite" });
    var submit = el("button", { type: "submit", class: "ssg-comments-submit" }, t.submit);
    form.appendChild(submit);
    form.appendChild(status);

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      submit.disabled = true;
      status.textContent = t.sending;
      var token = "";
      var tw = form.querySelector('[name="cf-turnstile-response"]');
      if (tw) token = tw.value;
      fetch(cfg.api, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          url: url, author: name.value, email: email.value, body: body.value,
          token: token, published: published,
        }),
      })
        .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d }; }); })
        .then(function (res) {
          if (res.ok) {
            form.reset();
            status.textContent = t.thanks;
            if (onPosted) onPosted();
          } else {
            status.textContent = (res.d && res.d.error) || t.error;
          }
        })
        .catch(function () { status.textContent = t.network; })
        .finally(function () {
          submit.disabled = false;
          if (window.turnstile) { try { window.turnstile.reset(); } catch (e2) { /* ignore */ } }
        });
    });

    root.appendChild(form);
  }

  function loadTurnstileScript() {
    if (document.querySelector('script[src*="challenges.cloudflare.com/turnstile"]')) return;
    var s = document.createElement("script");
    s.src = "https://challenges.cloudflare.com/turnstile/v0/api.js";
    s.async = true;
    s.defer = true;
    document.head.appendChild(s);
  }

  function mount() {
    var root = document.getElementById("ssg-comments");
    if (!root) return;
    var url = root.getAttribute("data-url") || location.pathname;
    var published = root.getAttribute("data-published") || "";
    var cfg = readConfig();
    var t = strings(cfg);

    var q = cfg.api + "?url=" + encodeURIComponent(url);
    if (published) q += "&published=" + encodeURIComponent(published);

    fetch(q, { headers: { accept: "application/json" } })
      .then(function (r) { return r.ok ? r.json() : { comments: [], count: 0 }; })
      .then(function (data) {
        root.textContent = "";
        renderList(root, data, t);
        // A closed thread shows its history but no form.
        if (data.closed) {
          root.appendChild(el("p", { class: "ssg-comments-closed" }, t.closed));
          return;
        }
        if (cfg.turnstileSiteKey) loadTurnstileScript();
        renderForm(root, cfg, url, t, published, function () { /* comment pending; list unchanged until approved */ });
      })
      .catch(function () {
        root.textContent = "";
        if (cfg.turnstileSiteKey) loadTurnstileScript();
        renderForm(root, cfg, url, t, published);
      });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", mount);
  } else {
    mount();
  }
})();
