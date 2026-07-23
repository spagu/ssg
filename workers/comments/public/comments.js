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

  function readConfig() {
    var el = document.getElementById("ssg-comments-config");
    var cfg = { api: "/api/comments", order: "newest", turnstileSiteKey: "" };
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

  function renderList(root, data) {
    var list = el("ol", { class: "ssg-comments-list" });
    if (!data.comments.length) {
      root.appendChild(el("p", { class: "ssg-comments-empty" }, "No comments yet. Be the first."));
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
    root.appendChild(el("h2", { class: "ssg-comments-title" }, "Comments (" + data.count + ")"));
    root.appendChild(list);
  }

  function renderForm(root, cfg, url, onPosted) {
    var form = el("form", { class: "ssg-comments-form", novalidate: "" });
    form.appendChild(el("h3", null, "Leave a comment"));

    var name = el("input", { type: "text", name: "author", required: "", maxlength: "80", placeholder: "Name", "aria-label": "Name" });
    var email = el("input", { type: "email", name: "email", maxlength: "254", placeholder: "Email (optional, for your avatar — never shown)", "aria-label": "Email (optional)" });
    var body = el("textarea", { name: "body", required: "", rows: "4", maxlength: "5000", placeholder: "Your comment", "aria-label": "Comment" });
    form.appendChild(name);
    form.appendChild(email);
    form.appendChild(body);

    if (cfg.turnstileSiteKey) {
      form.appendChild(el("div", { class: "cf-turnstile", "data-sitekey": cfg.turnstileSiteKey }));
    }

    var status = el("p", { class: "ssg-comments-status", role: "status", "aria-live": "polite" });
    var submit = el("button", { type: "submit", class: "ssg-comments-submit" }, "Post comment");
    form.appendChild(submit);
    form.appendChild(status);

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      submit.disabled = true;
      status.textContent = "Sending…";
      var token = "";
      var tw = form.querySelector('[name="cf-turnstile-response"]');
      if (tw) token = tw.value;
      fetch(cfg.api, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          url: url, author: name.value, email: email.value, body: body.value, token: token,
        }),
      })
        .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d }; }); })
        .then(function (res) {
          if (res.ok) {
            form.reset();
            status.textContent = "Thanks — your comment is awaiting review.";
            if (onPosted) onPosted();
          } else {
            status.textContent = (res.d && res.d.error) || "Could not post your comment.";
          }
        })
        .catch(function () { status.textContent = "Network error — please try again."; })
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
    var cfg = readConfig();
    if (cfg.turnstileSiteKey) loadTurnstileScript();

    fetch(cfg.api + "?url=" + encodeURIComponent(url), { headers: { accept: "application/json" } })
      .then(function (r) { return r.ok ? r.json() : { comments: [], count: 0 }; })
      .then(function (data) {
        root.textContent = "";
        renderList(root, data);
        renderForm(root, cfg, url, function () { /* comment pending; list unchanged until approved */ });
      })
      .catch(function () {
        root.textContent = "";
        renderForm(root, cfg, url);
      });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", mount);
  } else {
    mount();
  }
})();
