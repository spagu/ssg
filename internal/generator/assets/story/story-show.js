/* ssg story — fullscreen presentation.
 * Opens from the stage or the Presentation button at the slide being read and
 * plays: an optional title card, the article's slides, an optional end card.
 * A header (brand) and a footer (sponsor / ad slot) frame every slide. On
 * close the article scrolls to where the show stopped. The narrator
 * (story-voice.js) drives it through ssgStory.show while it reads. */
(function () {
  "use strict";
  var S = window.ssgStory;
  if (!S || !S.slides.length) return;
  var cfg = S.cfg.slideshow || {};
  var L = S.labels;
  var list = [];            // what the show plays: title? + slides + end?
  var show, box, frames, dots, playBtn, sub, at = -1, timer = 0, playing = true, opener = null, narrated = false;
  // The title card is an intro: it stays up for its whole duration even when
  // the narrator is already past the headline. Slides asked for meanwhile wait.
  var holdUntil = 0, pending = -1, holdTimer = 0;

  function interval(i) { return S.seconds(list[i].duration, S.seconds(cfg.interval, 6)); }
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  // card builds the title and end slides: text over an optional image.
  function card(c, kind) {
    var f = el("figure", "story-frame story-card story-card-" + kind);
    if (c.image) { var img = el("img"); img.dataset.src = c.image; img.alt = ""; f.appendChild(img); }
    var t = el("div", "story-card-text");
    if (c.kicker) t.appendChild(el("p", "story-kicker", c.kicker));
    if (c.title) t.appendChild(el(kind === "title" ? "h1" : "h2", "", c.title));
    if (c.subtitle) t.appendChild(el("p", "story-subtitle", c.subtitle));
    if (c.byline) t.appendChild(el("p", "story-byline", c.byline));
    if (c.button && c.button.href) {
      var a = el("a", "story-btn story-cta", c.button.label || c.button.href);
      a.href = c.button.href;
      t.appendChild(a);
    }
    f.appendChild(t);
    if (c.style) Object.assign(f.style, c.style);
    return f;
  }

  // auto fills a title card from the page itself: <h1>, meta description, byline.
  function auto(v, fallback) { return v === "auto" || v === undefined ? fallback : v; }
  function titleCard() {
    var t = cfg.title;
    if (!t) return null;
    if (t === true) t = {};
    var h1 = document.querySelector("h1");
    var by = document.querySelector(S.cfg.bylineSelector || ".post-meta");
    var title = auto(t.title, h1 ? h1.textContent.trim() : document.title);
    return { kind: "title", duration: t.duration || "5s", image: cardImage(t.image),
      kicker: t.kicker, title: title,
      subtitle: auto(t.subtitle, lead(title)), byline: auto(t.byline, by ? by.textContent.replace(/\s+/g, " ").trim() : ""),
      style: t.style };
  }
  // cardImage picks a card's photo: "auto" (or none given) and "og" take the
  // page's share image, "first" the first photo in the article — for sites
  // whose share image has a play button or text baked in — anything else is
  // a URL.
  function cardImage(v) {
    if (v === "first") return (S.slides[0] && S.slides[0].image) || pageImage();
    return v === "auto" || v === "og" || v === undefined ? pageImage() : v;
  }
  // pageImage is the page's share image (og:image), else the first slide.
  function pageImage() {
    var og = document.querySelector('meta[property="og:image"]');
    return og && og.content ? og.content : (S.slides[0] && S.slides[0].image) || "";
  }
  // lead makes a subtitle from the meta description: its first sentence,
  // without the title some sites repeat in front of it ("Babka – Dzisiaj…").
  function lead(title) {
    var meta = document.querySelector('meta[name="description"]');
    var d = meta ? meta.content.replace(/\s+/g, " ").trim() : "";
    if (title && d.toLowerCase().indexOf(title.toLowerCase()) === 0) d = d.slice(title.length).replace(/^[\s–—:-]+/, "");
    var first = (d.match(/^.+?[.!?](?=\s|$)/) || [d])[0];
    return first.length > 160 ? first.slice(0, 157).replace(/\s+\S*$/, "") + "…" : first;
  }

  // brand renders the header: logo, name, optional link.
  function brand(h) {
    var bar = el("header", "story-show-head");
    var wrap = h.link ? el("a") : el("span");
    if (h.link) wrap.href = h.link;
    if (h.logo) { var img = el("img"); img.src = h.logo; img.alt = h.text || ""; wrap.appendChild(img); }
    if (h.text) wrap.appendChild(el("span", "", h.text));
    bar.appendChild(wrap);
    if (h.style) Object.assign(bar.style, h.style);
    return bar;
  }

  // ad renders the footer slot. `html` is the site's own ad code (trusted
  // config, like a theme partial); otherwise image/text/link make a banner.
  // The label is the disclosure ad rules ask for.
  // With `items:` the slot rotates through several entries (an e-book, a
  // YouTube channel…) every `rotate` seconds; each may carry its own label.
  // Links are rel="sponsored" unless an entry says sponsored: false (the
  // site's own channel or product).
  function adItem(f, label) {
    var item = el("div", "story-ad-item");
    var lab = f.label === undefined ? label : f.label;
    if (lab !== false) item.appendChild(el("small", "story-ad-label", lab || "Advertisement"));
    if (f.html) { var raw = el("div", "story-ad-body"); raw.innerHTML = f.html; item.appendChild(raw); return item; }
    var a = el(f.link ? "a" : "div", "story-ad-body");
    if (f.link) { a.href = f.link; a.rel = f.sponsored === false ? "noopener" : "sponsored noopener"; a.target = "_blank"; }
    if (f.image) { var img = el("img"); img.src = f.image; img.alt = f.alt || ""; a.appendChild(img); }
    if (f.text) a.appendChild(el("span", "", f.text));
    item.appendChild(a);
    return item;
  }
  function ad(f) {
    var slot = el("aside", "story-ad");
    slot.setAttribute("aria-label", f.label || "Advertisement");
    var items = (f.items || [f]).map(function (it) { var n = adItem(it, f.label); slot.appendChild(n); return n; });
    var now = 0;
    items[0].classList.add("is-on");
    if (items.length > 1) {
      var paused = false;
      slot.addEventListener("mouseenter", function () { paused = true; });
      slot.addEventListener("mouseleave", function () { paused = false; });
      slot.addEventListener("focusin", function () { paused = true; });
      slot.addEventListener("focusout", function () { paused = false; });
      setInterval(function () {
        if (paused || document.hidden) return;
        items[now].classList.remove("is-on");
        now = (now + 1) % items.length;
        items[now].classList.add("is-on");
      }, S.seconds(f.rotate, 8) * 1000);
    }
    if (f.style) Object.assign(slot.style, f.style);
    return slot;
  }

  function build() {
    var title = titleCard();
    list = (title ? [title] : []).concat(S.slides);
    if (cfg.end) {
      var endCard = Object.assign({ kind: "end", duration: "8s" }, cfg.end);
      if (endCard.image) endCard.image = cardImage(endCard.image);
      list.push(endCard);
    }
    show = el("div", "story-show");
    show.setAttribute("role", "dialog");
    show.setAttribute("aria-modal", "true");
    show.setAttribute("aria-label", S.cfg.title || document.title);
    show.hidden = true;
    if (cfg.header) show.appendChild(brand(cfg.header));
    box = el("div", "story-frames");
    frames = list.map(function (s) {
      var f = s.kind ? card(s, s.kind) : S.frame(s);
      if (!s.kind) {
        // A blurred copy of the photo fills the sides when it is fitted to
        // the height, instead of black bars.
        var back = el("img", "story-backdrop");
        back.alt = "";
        back.dataset.src = s.image;
        f.insertBefore(back, f.firstChild);
        var img = f.querySelector("img:not(.story-backdrop)");
        img.addEventListener("load", function () { fit(f, img, s.fit || cfg.fit || "auto"); });
      }
      box.appendChild(f);
      return f;
    });
    sub = el("p", "story-sub");         // narrator subtitles
    sub.setAttribute("aria-hidden", "true");
    box.appendChild(sub);
    show.appendChild(box);
    if (cfg.footer) show.appendChild(ad(cfg.footer));
    var bar = el("div", "story-show-bar");
    dots = el("div", "story-dots");
    list.forEach(function (s, i) {
      var d = el("button");
      d.type = "button";
      d.setAttribute("aria-label", (i + 1) + ". " + (s.title || s.label || s.caption || ""));
      d.addEventListener("click", function () { go(i); });
      dots.appendChild(d);
    });
    var prev = S.button(L.prev, "M15.4 7.4 14 6l-6 6 6 6 1.4-1.4L10.8 12z", function () { go(at - 1); });
    var next = S.button(L.next, "M8.6 16.6 10 18l6-6-6-6-1.4 1.4 4.6 4.6z", function () { go(at + 1); });
    playBtn = S.button(L.pause, "M6 5h4v14H6zm8 0h4v14h-4z", function () { setPlaying(!playing); });
    var close = S.button(L.close, "M19 6.4 17.6 5 12 10.6 6.4 5 5 6.4 10.6 12 5 17.6 6.4 19l5.6-5.6 5.6 5.6 1.4-1.4-5.6-5.6z", end);
    [prev, playBtn, next].forEach(function (b) { bar.appendChild(b); });
    bar.appendChild(dots);
    (S.extraButtons || []).forEach(function (make) { bar.appendChild(make("show")); });
    if (S.audio) {
      var snd = S.button(L.sound, "M3 9v6h4l5 5V4L7 9H3zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4z", function () {
        S.sound(!S.soundOn()); snd.setAttribute("aria-pressed", S.soundOn() ? "true" : "false");
      });
      snd.dataset.sound = "";
      bar.appendChild(snd);
      document.addEventListener("ssg-story", function (e) {
        if (e.detail.type === "sound") snd.setAttribute("aria-pressed", e.detail.on ? "true" : "false");
      });
    }
    bar.appendChild(close);
    show.appendChild(bar);
    // Taps, holds and swipes on the photo are story-touch.js's; without it
    // a click moves on.
    box.addEventListener("click", function (e) { if (!S.touch && !e.target.closest("a")) go(at + 1); });
    document.body.appendChild(show);
    addEventListener("resize", function () { if (!show.hidden) refit(); }); // rotating a phone changes the answer
    show.addEventListener("keydown", keys);
    document.addEventListener("fullscreenchange", function () {
      if (!document.fullscreenElement && !show.hidden) end();
    });
  }

  // fit decides how a photo meets the screen. cover fills it (and crops);
  // contain fits the height. auto covers only when the photo has the pixels
  // and roughly the shape of the screen — a small or differently shaped
  // photo stretched to fill a desktop looks soft, so it is fitted instead.
  function fit(f, img, mode) {
    var cover = mode === "cover";
    if (mode === "auto" && img.naturalWidth) {
      var r = box.getBoundingClientRect(), dpr = Math.min(window.devicePixelRatio || 1, 2);
      var scale = Math.max(r.width / img.naturalWidth, r.height / img.naturalHeight) * dpr;
      var shape = (img.naturalWidth / img.naturalHeight) / (r.width / r.height);
      cover = scale <= 1.5 && shape > 0.7 && shape < 1.45;
    }
    f.classList.toggle("is-cover", cover);
  }
  function refit() {
    frames.forEach(function (f, i) {
      var img = f.querySelector("img:not(.story-backdrop)");
      if (!list[i].kind && img && img.complete) fit(f, img, list[i].fit || cfg.fit || "auto");
    });
  }

  function keys(e) {
    var k = e.key;
    if (k === "ArrowRight" || k === "PageDown") go(at + 1);
    else if (k === "ArrowLeft" || k === "PageUp") go(at - 1);
    else if (k === " " && e.target.tagName !== "SELECT") setPlaying(!playing);
    else if (k === "Escape") end();
    else return;
    e.preventDefault();
  }

  function setPlaying(on) {
    playing = on;
    show.classList.toggle("is-paused", !on || narrated);
    playBtn.lastChild.textContent = on ? L.pause : L.play;
    schedule();
  }

  // While the narrator reads, its voice sets the pace, not the timer.
  function schedule() {
    clearTimeout(timer);
    if (at < 0 || narrated) return;
    if (playing && at < list.length - 1) timer = setTimeout(function () { go(at + 1, "timer"); }, interval(at) * 1000);
    else if (playing && cfg.loop) timer = setTimeout(function () { go(0, "timer"); }, interval(at) * 1000);
  }

  // go shows slide i. `by` says who asked — "user" (buttons, dots, keys,
  // taps), "narrator" or "timer" — so the narrator can follow the reader's
  // hand without fighting its own moves.
  function go(i, by) {
    if (i < 0 || i >= list.length || i === at) return;
    S.swap(box, frames, at, i, list[i].effect || cfg.transition);
    at = i;
    show.classList.toggle("is-card", !!list[i].kind); // title/end cards get the whole screen
    Array.prototype.forEach.call(dots.children, function (d, n) {
      d.className = n < i ? "is-done" : n === i ? "is-now" : "";
      d.innerHTML = n === i ? "<i></i>" : "";
    });
    show.style.setProperty("--story-interval", interval(i) + "s");
    clearTimeout(holdTimer);
    holdUntil = list[i].kind === "title" ? Date.now() + interval(i) * 1000 : 0;
    if (S.refresh) S.refresh();
    schedule();
    S.emit("slide", { index: i, total: list.length, kind: list[i].kind || "slide", by: by || "user", el: list[i].el || null,
      label: list[i].title || list[i].label || list[i].caption || "" });
    if (i === list.length - 1) S.emit("complete", { total: list.length });
  }

  // slideFromNarrator moves to an article slide, after the intro if it is up.
  function slideFromNarrator(i) {
    if (i < 0) return;
    var wait = holdUntil - Date.now();
    if (wait <= 0) { go(i + offset(), "narrator"); return; }
    pending = i;
    clearTimeout(holdTimer);
    holdTimer = setTimeout(function () { holdUntil = 0; go(pending + offset(), "narrator"); }, wait);
  }

  // offset converts an article slide index into the show's own list.
  function offset() { return list.length && list[0].kind === "title" ? 1 : 0; }

  function begin(start, fromTitle) {
    if (!show) build();
    opener = document.activeElement;
    show.hidden = false;
    document.documentElement.style.overflow = "hidden";
    // Pinned widgets (the recipe) come along. A fullscreen element sits in
    // the browser's top layer, above every z-index, so they move inside it.
    document.documentElement.classList.add("story-showing");
    document.querySelectorAll(".story-recipe").forEach(function (w) { show.appendChild(w); });
    if (show.requestFullscreen) show.requestFullscreen().catch(function () {});
    at = -1;
    frames.forEach(function (f) { f.classList.remove("is-in", "is-out"); }); // from the last time it was open
    requestAnimationFrame(refit);
    setPlaying(cfg.autoplay !== false && !S.reduced);
    go(fromTitle || !start ? 0 : start + offset(), "open");
    narrated = !!(S.narrator && S.narrator.reading());
    if (narrated) setPlaying(playing);
    if (S.audio && S.cfg.audio.withPresentation !== false && !S.soundOn()) S.sound(true);
    var snd = show.querySelector("[data-sound]");
    if (snd) snd.setAttribute("aria-pressed", S.soundOn() ? "true" : "false");
    show.querySelector(".story-btn").focus({ preventScroll: true });
    S.emit("open", { index: at, total: list.length });
  }

  function end() {
    if (show.hidden) return;
    clearTimeout(timer);
    show.hidden = true;
    document.documentElement.style.overflow = "";
    document.documentElement.classList.remove("story-showing");
    show.querySelectorAll(".story-recipe").forEach(function (w) { document.body.appendChild(w); });
    if (document.fullscreenElement) document.exitFullscreen().catch(function () {});
    var s = list[at];
    if (s && s.el && cfg.returnToText !== false) {
      s.el.scrollIntoView({ block: "center", behavior: S.reduced ? "auto" : "smooth" });
    }
    if (opener && opener.focus) opener.focus({ preventScroll: true });
    if (S.onShowEnd) S.onShowEnd();
    S.emit("close", { index: at, total: list.length });
  }

  // The same sponsor slot sits under the reading stage, where readers spend
  // most of their time; `footer.reading: false` keeps it to the show.
  if (cfg.footer && cfg.footer.reading !== false && S.stage.parentNode) {
    var under = ad(cfg.footer);
    under.classList.add("story-ad-reading");
    document.body.appendChild(under);
  }

  window.ssgStoryShow = begin;
  S.show = {
    open: function () { return !!show && !show.hidden; },
    // current is the article element of the slide on screen (a card has none).
    current: function () { return list[at] && list[at].el ? list[at].el : null; },
    slide: slideFromNarrator,
    intro: function () { var t = list[0] && list[0].kind === "title" ? list[0] : null; return t ? t.subtitle || "" : ""; },
    finish: function () { if (list.length) go(list.length - 1, "narrator"); },
    // plan is what the show plays, as data: the build turns the same list
    // into the AMP Web Story and the 9:16 share pictures, so all three agree.
    // Each entry also carries what the narrator reads while it is on screen
    // (`text`) and the widgets anchored in that stretch of the article.
    plan: function () {
      if (!show) build();
      var off = offset(), texts = list.map(function () { return []; }), wids = list.map(function () { return []; });
      function owner(el) { return Math.max(0, S.slideFor(el) + off); } // before the first photo: the title card
      (S.narrator ? S.narrator.blocks() : []).forEach(function (b) { texts[owner(b)].push(S.readable(b)); });
      (S.widgets || []).forEach(function (w) {
        var c = w.cfg;
        if (!w.at || c.always || c.type === "recipe") return;
        wids[owner(w.at)].push({ type: c.type || "card", stat: c.stat || "", title: c.title || "", text: c.text || "" });
      });
      return list.map(function (s, i) {
        var lead = s.kind === "title" ? [s.title, s.subtitle] : s.kind === "end" ? [s.title, s.subtitle] : [];
        return { kind: s.kind || "slide", image: s.image || "", title: s.title || "", subtitle: s.subtitle || "",
          caption: s.caption || s.label || "", alt: s.alt || "", duration: interval(i),
          button: s.button || null, text: lead.concat(texts[i]).filter(Boolean), widgets: wids[i] };
      });
    },
    // for gestures, deep links and lock-screen controls
    go: function (i, by) { go(i, by); },
    at: function () { return at; },
    total: function () { return list.length; },
    box: function () { return box; },
    bar: function () { return show && show.querySelector(".story-show-bar"); },
    playing: function () { return playing; },
    play: function (on) { if (show) setPlaying(on); },
    close: function () { if (show) end(); },
    narrate: function (on) { narrated = on; if (show) { setPlaying(playing); if (!on) S.show.subtitle(""); } },
    subtitle: function (text) {
      if (!sub) return;
      if (holdUntil > Date.now()) text = ""; // the intro card speaks for itself
      sub.textContent = text;
      sub.classList.toggle("is-on", !!text);
    }
  };
})();
