/* ssg story — reading companion.
 * Reads <script type="application/json" id="ssg-story"> (the YAML, converted
 * at build time) and adds: a reading bar with time left, an image stage that
 * follows the paragraph being read, timed widgets, background audio, and a
 * button that opens the fullscreen presentation (story-show.js). */
(function () {
  "use strict";
  var src = document.getElementById("ssg-story");
  if (!src) return;
  var cfg = JSON.parse(src.textContent);
  var root = document.documentElement;
  var content = document.querySelector(cfg.content || "article");
  if (!content) return;
  var column = document.querySelector(cfg.column || cfg.content || "article");
  var reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  var labels = Object.assign({ left: "{n} min left", done: "Finished", present: "Presentation",
    stage: "Photographs from the article", tools: "Story",
    sound: "Sound", close: "Close", pause: "Pause", play: "Play", prev: "Previous", next: "Next" }, cfg.labels);
  var trigger = cfg.trigger || 0.45; // the reading line, as a fraction of the viewport

  // seconds parses "6s", "800ms" or a bare number of seconds.
  function seconds(v, dflt) {
    if (v == null) return dflt;
    if (typeof v === "number") return v;
    var m = /^([\d.]+)\s*(ms|s)?$/.exec(String(v).trim());
    return m ? parseFloat(m[1]) / (m[2] === "ms" ? 1000 : 1) : dflt;
  }

  // find resolves an anchor: a CSS selector ("#latitude"), or "text:…" for
  // the first block whose text contains it — no ids needed in the Markdown.
  function find(a) {
    if (!a) return null;
    if (a.indexOf("text:") === 0) {
      var needle = a.slice(5).trim().toLowerCase();
      var blocks = content.querySelectorAll("p, li, h2, h3, h4, blockquote, figure, td");
      for (var i = 0; i < blocks.length; i++) {
        if (blocks[i].textContent.toLowerCase().indexOf(needle) !== -1) return blocks[i];
      }
      return null;
    }
    try { return content.querySelector(a) || document.querySelector(a); } catch (e) { return null; }
  }
  function top(el) { return el.getBoundingClientRect().top; }

  // focus is where the reader is when scrolling does not say it: the block
  // the narrator is reading, or the slide on screen in the presentation.
  // null means "use the reading line".
  function focus() {
    var S = window.ssgStory || {};
    if (S.narrator && S.narrator.reading() && S.narrator.block()) return S.narrator.block();
    if (S.show && S.show.open()) return S.show.current();
    return null;
  }

  // readable returns what a person reads in an element: no inline scripts
  // (WordPress puts ad code inside <p>), styles, ads, embeds or bare URLs —
  // textContent alone hands all of that to the narrator and the word count.
  var NOT_TEXT = "script, style, noscript, template, iframe, object, embed, svg, ins, button, select, .screen-reader-text";
  var URL_RE = /\b(?:https?:\/\/|www\.)\S+/gi;
  function readable(el) {
    var c = el.cloneNode(true);
    c.querySelectorAll(NOT_TEXT).forEach(function (n) { n.remove(); });
    return c.textContent.replace(URL_RE, " ").replace(/\s+/g, " ").trim();
  }
  function line() { return innerHeight * trigger; }

  // imgSrc reads an image's real URL; lazy loaders keep it in data-src and
  // put a placeholder in src.
  function imgSrc(img) { return img.getAttribute("data-src") || img.getAttribute("src"); }

  // ---- per-site tweaks: elements hidden in story mode, extra CSS -----------
  var extra = (cfg.hide || []).map(function (sel) { return "html.story-side " + sel + "{display:none!important}"; }).join("");
  if (cfg.css || extra) {
    var st = document.createElement("style");
    st.textContent = extra + (cfg.css || "");
    document.head.appendChild(st);
  }

  // ---- theme: every YAML theme key becomes a --story-* property ----------
  Object.keys(cfg.theme || {}).forEach(function (k) {
    root.style.setProperty("--story-" + k.replace(/_/g, "-").replace(/[A-Z]/g, function (c) { return "-" + c.toLowerCase(); }), cfg.theme[k]);
  });
  if (cfg.safeTop) root.style.setProperty("--story-safe-top", cfg.safeTop);
  root.style.setProperty("--story-ms", (reduced ? 0 : seconds(cfg.transitionTime, 0.9) * 1000) + "ms");

  // ---- slides --------------------------------------------------------------
  var slides = (cfg.slides || []).map(function (s) {
    return Object.assign({}, s, { el: find(s.at) });
  });
  // Inline photos join the stage on their own: <figure>s, and the bare
  // <p><a><img></a></p> older posts use. figureSkip lists selectors that are
  // not story photos (an e-book cover, a video thumbnail); tiny images
  // (icons) are skipped by size.
  var skip = (cfg.figureSkip || []).join(",");
  function storyImg(img) {
    if (skip && img.matches(skip)) return false;
    var w = parseInt(img.getAttribute("width"), 10);
    return !(w && w < (cfg.figureMinWidth || 300));
  }
  if (cfg.figures !== false) {
    var hosts = [];
    content.querySelectorAll("img").forEach(function (img) {
      if (!storyImg(img)) return;
      var f = img.closest("figure");
      if (!f) { // a paragraph holding nothing but the photo
        var p = img.closest("p");
        if (!p || !content.contains(p) || readable(p)) return;
        p.classList.add("story-fig");
        f = p;
      }
      if (hosts.indexOf(f) === -1) hosts.push(f);
    });
    hosts.forEach(function (f) {
      var img = Array.prototype.filter.call(f.querySelectorAll("img"), storyImg)[0];
      if (!img || slides.some(function (s) { return s.image === imgSrc(img); })) return;
      var cap = f.querySelector("figcaption");
      // A marker before the figure keeps its place: a figure hidden on wide
      // screens (hideFigures) has no position of its own.
      var mark = document.createElement("span");
      mark.className = "story-mark";
      f.parentNode.insertBefore(mark, f);
      slides.push({ el: mark, image: imgSrc(img), alt: img.alt, caption: cap ? cap.textContent : "" });
    });
  }
  slides = slides.filter(function (s) { return s.el && s.image; });
  slides.sort(function (a, b) { return a.el.compareDocumentPosition(b.el) & 2 ? 1 : -1; });
  var pageTitle = (document.querySelector("h1") || {}).textContent || "";
  slides.forEach(function (s) {
    if (s.label == null) { // the nearest heading names the slide: <h2>/<h3>, or a bold lead line
      for (var n = s.el, hops = 0; n && n !== content && hops < 12; n = n.previousElementSibling || n.parentElement, hops++) {
        if (/^H[23]$/.test(n.tagName)) { s.label = n.textContent.replace(/:.*/, "").trim(); break; }
        var lead = n.tagName === "P" && n.firstElementChild;
        if (lead && lead.tagName === "STRONG" && n.textContent.trim().indexOf(lead.textContent.trim()) === 0) {
          s.label = lead.textContent.replace(/[:\s–-]+$/, "").trim(); break;
        }
      }
      // A post that repeats its own title in bold names nothing.
      if (s.label && s.label.toLowerCase() === pageTitle.trim().toLowerCase()) s.label = "";
    }
  });

  // frame builds one <figure> for a slide; story-show.js reuses it.
  function frame(s, eager) {
    var f = document.createElement("figure");
    f.className = "story-frame";
    var img = document.createElement("img");
    img.alt = s.alt || s.caption || "";
    img.decoding = "async";
    if (eager) img.src = s.image; else img.dataset.src = s.image;
    if (s.focus) img.style.objectPosition = s.focus;
    f.appendChild(img);
    if (s.caption || s.label) {
      var c = document.createElement("figcaption");
      if (s.label) { var b = document.createElement("b"); b.textContent = s.label; c.appendChild(b); }
      c.appendChild(document.createTextNode(s.caption || ""));
      f.appendChild(c);
    }
    return f;
  }
  function load(f) {
    if (!f) return;
    f.querySelectorAll("img[data-src]").forEach(function (i) { i.src = i.dataset.src; delete i.dataset.src; });
  }

  // swap moves the "is-in" state between frames of one container.
  function swap(box, frames, from, to, effect) {
    box.className = box.className.replace(/\bstory-fx-\S+/g, "").trim() + " story-fx-" + (effect || cfg.transition || "crossfade");
    load(frames[to]); load(frames[to + 1]);
    if (frames[from]) {
      var old = frames[from];
      old.classList.remove("is-in"); old.classList.add("is-out");
      setTimeout(function () { old.classList.remove("is-out"); }, seconds(cfg.transitionTime, 0.9) * 1000 + 50);
    }
    if (frames[to]) {
      var img = frames[to].querySelector("img");
      if (img) { img.style.animation = "none"; void img.offsetWidth; img.style.animation = ""; } // restart Ken Burns
      frames[to].classList.add("is-in");
    }
  }

  // Everything story mode adds to the page sits in one named region, so a
  // screen reader finds it as a landmark and no content floats outside one.
  // Its children are fixed-positioned; the region itself takes no space.
  var layer = document.createElement("div");
  layer.className = "story-layer";
  layer.setAttribute("role", "region");
  layer.setAttribute("aria-label", labels.tools);
  document.body.appendChild(layer);

  // ---- stage ---------------------------------------------------------------
  var stageCfg = cfg.stage || {};
  var stage = document.createElement("div");
  stage.className = "story-stage";
  stage.setAttribute("role", "img");
  stage.setAttribute("aria-label", labels.stage);
  stage.tabIndex = 0;
  var stageFrames = slides.map(function (s) { var f = frame(s); stage.appendChild(f); return f; });
  if (slides.length) layer.appendChild(stage);
  column.classList.add("story-content-host");
  if (stageCfg.width) root.style.setProperty("--story-stage-width", stageCfg.width);
  if (stageCfg.leftSpace) root.style.setProperty("--story-left-space", stageCfg.leftSpace);
  if (stageCfg.side === "left") root.classList.add("story-left");
  if (stageCfg.hideFigures !== false) root.classList.add("story-hide-figures");
  function layout() {
    var wide = innerWidth >= (stageCfg.minWidth || 1100);
    root.classList.toggle("story-side", wide && slides.length > 0);
    root.classList.toggle("story-mini", !wide);
  }

  var current = -1;
  function activeSlide() {
    var idx = -1;
    for (var i = 0; i < slides.length; i++) if (top(slides[i].el) < line()) idx = i;
    return idx;
  }

  // ---- widgets -------------------------------------------------------------
  var widgets = (cfg.widgets || []).map(function (w) {
    var el = document.createElement("div"); // not <aside>: one landmark per tip would be noise
    el.className = "story-widget";
    el.setAttribute("role", "note");
    el.dataset.type = w.type || "card";
    el.dataset.slot = (w.position || "bottom-right").split("-")[0];
    el.setAttribute("aria-live", "polite");
    var render = (window.ssgStoryWidgets || {})[w.type];
    var update = render ? render(w, el, { find: find, content: content, top: top, line: line, focus: focus }) : null;
    if (!render && w.stat) { var st = document.createElement("div"); st.className = "story-stat"; st.textContent = w.stat; el.appendChild(st); }
    if (!render && w.title) { var h = document.createElement("h3"); h.textContent = w.title; el.appendChild(h); }
    if (!render && w.text) { var p = document.createElement("p"); p.textContent = w.text; el.appendChild(p); }
    var pos = (w.position || "bottom-right").split("-");
    var dx = w.x || "var(--story-stage-gap)", dy = w.y || (pos[0] === "top" ? cfg.safeTop || "1.5rem" : "5rem");
    el.style[pos.indexOf("left") !== -1 ? "left" : "right"] = dx;
    if (pos[0] === "center") { el.style.top = "50%"; el.style.translate = "0 -50%"; } else el.style[pos[0]] = dy;
    if (w.width) el.style.width = w.width;
    Object.assign(el.style, w.style || {});
    layer.appendChild(el);
    var at = w.always ? content : find(w.at), until = w.until ? find(w.until) : null;
    if (at && !until && !w.always) { // default: until the next heading after the anchor
      var all = Array.prototype.slice.call(content.querySelectorAll("h2, h3"));
      until = all.filter(function (h) { return at.compareDocumentPosition(h) & 4; })[0] || null;
      if (!until) { // a post without headings: until the next slide
        var nextSlide = slides.filter(function (sl) { return at.compareDocumentPosition(sl.el) & 4; })[0];
        until = nextSlide ? nextSlide.el : null;
      }
    }
    return { cfg: w, el: el, at: at, until: until, update: update, shown: false, spent: false, timer: 0 };
  }).filter(function (w) { return w.at; });

  // One widget per position: when two are in range at the same spot, the one
  // anchored later in the text wins, so a new widget replaces the old one.
  function updateWidgets() {
    var winner = {};
    widgets.forEach(function (w) {
      w.inRange = w.cfg.always || top(w.at) < line() && (!w.until || top(w.until) > line());
      if (w.inRange) winner[w.cfg.position || "bottom-right"] = w;
    });
    widgets.forEach(function (w) {
      if (w.update) w.update();
      var inRange = w.inRange && winner[w.cfg.position || "bottom-right"] === w;
      if (!inRange) { w.spent = false; clearTimeout(w.timer); w.el.classList.remove("is-on"); w.shown = false; return; }
      if (w.shown || w.spent) return;
      w.shown = true;
      w.timer = setTimeout(function () {
        w.el.classList.add("is-on");
        var d = seconds(w.cfg.duration, 0);
        if (d) w.timer = setTimeout(function () { w.el.classList.remove("is-on"); w.spent = true; }, d * 1000);
      }, seconds(w.cfg.delay, 0) * 1000);
    });
  }

  // ---- progress and time left ---------------------------------------------
  var prog = cfg.progress || {};
  var words = (readable(content).match(/\S+/g) || []).length;
  var bar = document.createElement("div"), fill = document.createElement("i");
  bar.className = "story-bar"; bar.dataset.pos = prog.position || "top"; bar.appendChild(fill);
  var time = document.createElement("div");
  time.className = "story-time"; time.setAttribute("aria-live", "off");
  if (prog.bar !== false) layer.appendChild(bar);
  if (prog.timeLeft !== false) layer.appendChild(time);
  function updateProgress() {
    var r = content.getBoundingClientRect();
    var p = Math.min(1, Math.max(0, (line() - r.top) / r.height));
    fill.style.transform = "scaleX(" + p + ")";
    var mins = Math.ceil(words * (1 - p) / (prog.wordsPerMinute || 230));
    time.textContent = p >= 0.995 ? labels.done : labels.left.replace("{n}", Math.max(1, mins));
    time.classList.toggle("is-on", r.top < line() && p < 1);
  }

  // ---- audio ---------------------------------------------------------------
  var audio = null;
  // audio may be one entry or a list of entries with date windows
  // (from/to as "MM-DD", wrapping over New Year); the first that matches
  // today wins, an entry without dates matches always. An entry plays
  // `src`, or a `playlist` — shuffled with `shuffle: true`, never the same
  // track twice in a row.
  // A festival that moves every year (Easter, Diwali, Holi) takes full
  // dates, "2026-11-03"; an entry may list several windows in `dates`.
  function inWindow(from, to) {
    var d = new Date(), md = ("0" + (d.getMonth() + 1)).slice(-2) + "-" + ("0" + d.getDate()).slice(-2);
    var full = (from || to || "").length > 5, now = full ? d.getFullYear() + "-" + md : md;
    from = from || (full ? "0000-01-01" : "01-01");
    to = to || (full ? "9999-12-31" : "12-31");
    return from <= to ? now >= from && now <= to : now >= from || now <= to;
  }
  function today(item) {
    if (item.dates) return item.dates.some(function (w) { return inWindow(w.from, w.to); });
    if (!item.from && !item.to) return true;
    return inWindow(item.from, item.to);
  }
  var track = [].concat(cfg.audio || []).filter(today)[0] || null;
  cfg.audio = track; // the rest of the runtime sees the one that plays
  if (track && (track.src || (track.playlist || []).length)) {
    var tracks = track.playlist || [track.src], last = -1;
    var pick = function () {
      if (tracks.length === 1) return 0;
      if (!track.shuffle) return (last + 1) % tracks.length;
      var i;
      do { i = Math.floor(Math.random() * tracks.length); } while (i === last);
      return i;
    };
    audio = new Audio();
    audio.preload = "none";
    audio.loop = tracks.length === 1 && track.loop !== false;
    last = pick();
    audio.src = tracks[last];
    audio.addEventListener("ended", function () {
      if (tracks.length < 2) return;
      last = pick();
      audio.src = tracks[last];
      audio.play().catch(function () {});
    });
  }
  function sound(on) {
    if (!audio) return;
    var vol = cfg.audio.volume == null ? 0.4 : cfg.audio.volume;
    // A browser that blocks sound before the visitor touches the page
    // rejects play(); the button then says "off", as it is.
    if (on) {
      audio.volume = 0;
      audio.play().catch(function () { soundBtn.setAttribute("aria-pressed", "false"); emit("sound", { on: false }); });
      fade(vol);
    }
    else fade(0, function () { audio.pause(); });
    soundBtn.setAttribute("aria-pressed", on ? "true" : "false");
    emit("sound", { on: on });
  }
  function fade(to, done) { // a short ramp, so the music never starts with a jolt
    var from = audio.volume, t0 = performance.now(), ms = reduced ? 1 : 1200;
    (function step(now) {
      var k = Math.min(1, (now - t0) / ms);
      audio.volume = Math.min(1, Math.max(0, from + (to - from) * k));
      if (k < 1) requestAnimationFrame(step); else if (done) done();
    })(t0);
  }

  // ---- controls ------------------------------------------------------------
  function button(text, icon, onClick) {
    var b = document.createElement("button");
    b.type = "button"; b.className = "story-btn";
    b.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="' + icon + '"/></svg>';
    b.appendChild(document.createTextNode(text));
    b.addEventListener("click", onClick);
    return b;
  }
  var controls = document.createElement("div");
  controls.className = "story-controls";
  var soundBtn = button(labels.sound, "M3 9v6h4l5 5V4L7 9H3zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4z", function () {
    sound(soundBtn.getAttribute("aria-pressed") !== "true");
  });
  if (audio) { soundBtn.setAttribute("aria-pressed", "false"); controls.appendChild(soundBtn); }
  // present opens the show from its title card (or, with startAt: current,
  // at the slide being read) and lets the narrator read it, unless
  // slideshow.narrate is false.
  function present() {
    if (!window.ssgStoryShow) return;
    var sc = cfg.slideshow || {}, here = sc.startAt === "current";
    window.ssgStoryShow(here ? Math.max(0, current) : 0, !here);
    var n = window.ssgStory.narrator;
    if (n && sc.narrate !== false && !n.reading() && n.available()) n.start({ fromTop: !here });
  }
  if (slides.length) controls.appendChild(button(labels.present, "M3 5h18v11H3zM8 20h8v-2H8zM10 8v5l4-2.5z", present));
  layer.appendChild(controls);
  stage.addEventListener("click", present);
  stage.addEventListener("keydown", function (e) { if (e.key === "Enter") present(); });

  // ---- the scroll loop -------------------------------------------------------
  var queued = false;
  function tick() {
    queued = false;
    var idx = activeSlide();
    if (idx !== current) {
      swap(stage, stageFrames, current, idx, idx >= 0 && slides[idx].effect);
      current = idx;
      stage.classList.toggle("is-on", idx >= 0);
      stage.setAttribute("aria-label", (idx >= 0 && (slides[idx].alt || slides[idx].caption)) || labels.stage);
    }
    updateWidgets();
    updateProgress();
  }
  function queue() { if (!queued) { queued = true; requestAnimationFrame(tick); } }
  addEventListener("scroll", queue, { passive: true });
  addEventListener("resize", function () { layout(); queue(); });
  layout(); tick();

  // slideFor returns the slide a block of text belongs to: the last one
  // anchored at or before it (-1 before the first).
  function slideFor(block) {
    var idx = -1;
    slides.forEach(function (s, i) { if (s.el === block || (s.el.compareDocumentPosition(block) & 4)) idx = i; });
    return idx;
  }

  // emit tells the rest of the page what the story does — analytics, the
  // share button and the lock-screen controls listen for one "ssg-story"
  // event on document, so none of them reaches into another's code.
  function emit(type, detail) {
    document.dispatchEvent(new CustomEvent("ssg-story", { detail: Object.assign({ type: type }, detail) }));
  }

  window.ssgStory = { cfg: cfg, layer: layer, slides: slides, labels: labels, frame: frame, swap: swap, load: load, emit: emit, present: present, widgets: widgets,
    content: content, controls: controls, readable: readable, refresh: function () { queue(); }, stage: stage, slideFor: slideFor, line: line,
    seconds: seconds, sound: sound, audio: audio, button: button, reduced: reduced,
    soundOn: function () { return soundBtn.getAttribute("aria-pressed") === "true"; } };
})();
