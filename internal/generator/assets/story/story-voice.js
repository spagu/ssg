/* ssg story — narrator.
 * Reads the article aloud with the browser's voice (Web Speech API), the same
 * way ssg's `listen:` button does: one utterance per sentence, so Chrome's
 * ~15 s cut-off never bites, and the best voice for the page language first.
 * What it adds for a story: the text being read is highlighted and scrolled to
 * the reading line, so slides and widgets follow the voice; in the fullscreen
 * show it moves the slides and prints subtitles; background music ducks while
 * it speaks. The visitor picks the voice and speed; the choice is remembered.
 * The page's own Listen button ([data-ssg-listen]) starts the narrator too. */
(function () {
  "use strict";
  var S = window.ssgStory;
  if (!S || S.cfg.narrator === false || !("speechSynthesis" in window)) return;
  var cfg = Object.assign({ voice: "", rate: 1, pitch: 1, duck: 0.3, subtitles: true, title: true }, S.cfg.narrator);
  var L = Object.assign({ listen: "Listen", stop: "Stop", voice: "Voice", speed: "Speed", subtitles: "Subtitles" }, S.cfg.labels);
  var synth = window.speechSynthesis;
  var lang = (document.documentElement.lang || "en").toLowerCase().replace("_", "-");
  var KEY = "ssg-story-voice";
  var saved = {};
  try { saved = JSON.parse(localStorage.getItem(KEY)) || {}; } catch (e) { saved = {}; }

  // ---- voices: the same preference order as ssg's listen button ----------
  function norm(l) { return (l || "").toLowerCase().replace("_", "-"); }
  function sameLang(v) { return norm(v.lang).split("-")[0] === lang.split("-")[0]; }
  function score(v) {
    var n = v.name, s = 0;
    if (cfg.voice && n.toLowerCase().indexOf(cfg.voice.toLowerCase()) !== -1) s += 100;
    if (/Natural|Online|Neural|Google|Premium|Enhanced|Siri/i.test(n)) s += 10;
    if (norm(v.lang) === lang) s += 5;
    return s;
  }
  function voices() {
    return synth.getVoices().filter(sameLang).sort(function (a, b) { return score(b) - score(a); });
  }
  function voice() {
    var vs = voices();
    return vs.filter(function (v) { return v.voiceURI === saved.voice; })[0] || vs[0] || null;
  }

  // ---- what is read: headings, paragraphs, list items, quotes ------------
  var blocks = Array.prototype.filter.call(
    S.content.querySelectorAll("h2, h3, h4, p, li, blockquote"),
    function (b) {
      return !b.closest("figure, nav, .story-widget") && !b.querySelector("p, li") && S.readable(b);
    });
  // Abbreviations the pronunciation rules know ("e.g.", "np.", "łyż.") do
  // not end a sentence: their dots are hidden from the split and put back.
  var DOT = "\u2024";
  var abbr = Object.keys((S.cfg.narrator || {}).pronounce || {}).map(function (k) {
    return k.replace("{num}", " ").trim().split(/\s+/).pop();
  }).filter(function (w) { return /\.$/.test(w) && w.indexOf("|") < 0; })
    .sort(function (a, b) { return b.length - a.length; });
  var abbrRe = abbr.length ? new RegExp("(^|[^\\p{L}\\p{N}])(" + abbr.map(function (w) {
    return w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }).join("|") + ")", "gu") : null;
  function sentences(text) {
    text = text.replace(/\s+/g, " ").trim();
    if (abbrRe) text = text.replace(abbrRe, function (m, pre, w) { return pre + w.replace(/\./g, DOT); });
    return (text.match(/[^.!?]+[.!?]+["')\]]*\s*|[^.!?]+$/g) || [])
      .map(function (t) { return t.split(DOT).join(".").trim(); }).filter(Boolean);
  }

  // ---- state -----------------------------------------------------------------
  var reading = false, run = 0, idx = 0, baseVolume = null;
  var buttons = [];

  function mark(b) {
    S.content.querySelectorAll(".story-speaking").forEach(function (n) { n.classList.remove("story-speaking"); });
    if (b) b.classList.add("story-speaking");
  }
  function follow(b) {
    if (S.show && S.show.open()) { S.show.slide(S.slideFor(b)); return; }
    var y = b.getBoundingClientRect().top + scrollY - S.line() + 8;
    scrollTo({ top: y, behavior: S.reduced ? "auto" : "smooth" });
  }
  // Subtitles are on unless the config or the reader turned them off.
  function subsOn() { return cfg.subtitles && saved.subs !== false; }
  function subtitle(t) { if (S.show) S.show.subtitle(subsOn() ? t : ""); }
  function duck(on) {
    if (!S.audio || !S.soundOn()) return;
    if (on && baseVolume == null) { baseVolume = S.audio.volume; S.audio.volume = baseVolume * cfg.duck; }
    if (!on && baseVolume != null) { S.audio.volume = baseVolume; baseVolume = null; }
  }

  // say queues one utterance and calls next when it ends — unless the run
  // was stopped or restarted meanwhile (run changes).
  function say(text, onStart, next) {
    var my = run;
    var u = new SpeechSynthesisUtterance(S.pronounce ? S.pronounce(text) : text); // "100g" → "100 gramów"; the subtitle keeps what is written
    var v = voice();
    if (v) u.voice = v;
    u.lang = v ? v.lang : lang;
    u.rate = saved.rate || cfg.rate;
    u.pitch = cfg.pitch;
    u.onstart = function () { if (my === run) onStart(); };
    u.onend = function () { if (my === run && reading) next(); };
    u.onerror = function (e) { if (my === run && reading && e.error !== "interrupted" && e.error !== "canceled") next(); };
    synth.speak(u);
  }

  function readBlock(i) {
    if (i >= blocks.length) { // the end: the show moves on to its closing card
      if (S.show && S.show.open()) S.show.finish();
      stop();
      return;
    }
    idx = i;
    if (S.refresh) S.refresh(); // widgets that follow the reader (the recipe's step)
    var b = blocks[i];
    var parts = sentences(S.readable(b));
    mark(b);
    follow(b);
    (function part(k) {
      if (k >= parts.length) { readBlock(i + 1); return; }
      say(parts[k], function () { subtitle(parts[k]); }, function () { part(k + 1); });
    })(0);
  }

  // start reads from the block at the reading line, or from the title when
  // the reader has not reached the text yet.
  function start(opts) {
    var fromTop = !!(opts && opts.fromTop);
    run++;
    synth.cancel();
    reading = true;
    sync();
    duck(true);
    if (S.show) S.show.narrate(true);
    S.emit("narrator", { on: true });
    var from = 0;
    for (var i = 0; i < blocks.length; i++) if (blocks[i].getBoundingClientRect().top <= S.line()) from = i;
    if (fromTop) from = 0;
    var atTop = fromTop || (blocks.length && blocks[0].getBoundingClientRect().top > S.line());
    var h1 = document.querySelector("h1");
    if (atTop && cfg.title && h1) {
      // The intro: headline, then the title card's subtitle, with no
      // subtitles on screen — the card already shows both.
      var intro = S.show && S.show.open() ? S.show.intro() : "";
      say(h1.textContent.trim(), function () { subtitle(""); }, function () {
        if (intro) say(intro, function () { subtitle(""); }, function () { readBlock(0); });
        else readBlock(0);
      });
    } else readBlock(from);
  }
  function stop() {
    run++;
    reading = false;
    synth.cancel();
    mark(null);
    duck(false);
    if (S.show) S.show.narrate(false);
    S.emit("narrator", { on: false });
    sync();
  }
  function toggle() { if (reading) stop(); else start(); }

  // ---- controls: Listen + a voice/speed panel ---------------------------------
  function sync() {
    buttons.forEach(function (b) {
      b.setAttribute("aria-pressed", reading ? "true" : "false");
      b.lastChild.textContent = reading ? L.stop : L.listen;
    });
    document.querySelectorAll("[data-ssg-listen]").forEach(function (b) { b.setAttribute("aria-pressed", reading ? "true" : "false"); });
  }
  function panel() {
    var wrap = document.createElement("div");
    wrap.className = "story-voice-panel";
    wrap.hidden = true;
    var vSel = document.createElement("select");
    vSel.setAttribute("aria-label", L.voice);
    function fill() {
      vSel.innerHTML = "";
      var current = voice();
      voices().forEach(function (v) {
        var o = document.createElement("option");
        o.value = v.voiceURI;
        o.textContent = v.name.replace(/^Microsoft /, "") + " (" + v.lang + ")";
        o.selected = current && v.voiceURI === current.voiceURI;
        vSel.appendChild(o);
      });
      if (!vSel.options.length) vSel.appendChild(new Option("—", ""));
    }
    fill();
    synth.addEventListener && synth.addEventListener("voiceschanged", fill);
    var rSel = document.createElement("select");
    rSel.setAttribute("aria-label", L.speed);
    [0.8, 0.9, 1, 1.1, 1.25, 1.5].forEach(function (r) {
      var o = new Option(r + "×", r);
      o.selected = r === (saved.rate || cfg.rate);
      rSel.appendChild(o);
    });
    var subs = document.createElement("input");
    subs.type = "checkbox";
    subs.checked = subsOn();
    subs.disabled = !cfg.subtitles;
    subs.addEventListener("change", function () {
      saved.subs = subs.checked;
      try { localStorage.setItem(KEY, JSON.stringify(saved)); } catch (e) { /* private mode */ }
      if (!subs.checked) subtitle("");
    });
    function save() {
      saved = { voice: vSel.value, rate: parseFloat(rSel.value), subs: subs.checked };
      try { localStorage.setItem(KEY, JSON.stringify(saved)); } catch (e) { /* private mode: keep it for this page */ }
      if (reading) { var keep = idx; run++; synth.cancel(); readBlock(keep); } // new voice from this paragraph on
    }
    vSel.addEventListener("change", save);
    rSel.addEventListener("change", save);
    [[L.voice, vSel], [L.speed, rSel]].forEach(function (pair) {
      var l = document.createElement("label");
      l.appendChild(document.createTextNode(pair[0]));
      l.appendChild(pair[1]);
      wrap.appendChild(l);
    });
    var sl = document.createElement("label");
    sl.className = "story-voice-check";
    sl.appendChild(subs);
    sl.appendChild(document.createTextNode(" " + L.subtitles));
    wrap.appendChild(sl);
    return wrap;
  }
  function make() {
    var group = document.createElement("span");
    group.className = "story-voice";
    var listen = S.button(L.listen, "M3 9v6h4l5 5V4L7 9H3zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4zM14 3.2v2.1a7 7 0 0 1 0 13.4v2.1a9 9 0 0 0 0-17.6z", toggle);
    listen.setAttribute("aria-pressed", "false");
    buttons.push(listen);
    var p = panel();
    var gear = S.button("", "M12 15.5A3.5 3.5 0 1 1 12 8.5a3.5 3.5 0 0 1 0 7zm7.4-2.5a7.6 7.6 0 0 0 0-2l2.1-1.6-2-3.4-2.5 1a7.4 7.4 0 0 0-1.7-1L15 3.3h-4l-.4 2.7a7.4 7.4 0 0 0-1.7 1l-2.5-1-2 3.4L6.6 11a7.6 7.6 0 0 0 0 2l-2.1 1.6 2 3.4 2.5-1c.5.4 1.1.7 1.7 1l.4 2.7h4l.4-2.7c.6-.3 1.2-.6 1.7-1l2.5 1 2-3.4z", function () {
      p.hidden = !p.hidden;
      gear.setAttribute("aria-expanded", p.hidden ? "false" : "true");
    });
    gear.setAttribute("aria-label", L.voice);
    gear.setAttribute("aria-expanded", "false");
    group.appendChild(p);
    group.appendChild(gear);
    group.appendChild(listen);
    sync();
    return group;
  }

  S.controls.insertBefore(make(), S.controls.firstChild);
  (S.extraButtons = S.extraButtons || []).push(make);
  // The theme's Listen button now starts the narrator instead of the plain reader.
  document.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest("[data-ssg-listen]");
    if (!b) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    toggle();
  }, true);
  // The reader moved the show by hand while it reads: carry on from that
  // slide's text (the title card starts over, the end card stops).
  document.addEventListener("ssg-story", function (e) {
    var d = e.detail;
    if (d.type !== "slide" || d.by !== "user" || !reading) return;
    if (d.kind === "end") { stop(); return; }
    if (d.kind === "title") { start({ fromTop: true }); return; }
    var to = -1;
    for (var i = 0; i < blocks.length; i++) if (d.el === blocks[i] || (d.el.compareDocumentPosition(blocks[i]) & 4)) { to = i; break; }
    if (to < 0) return; // a photo after the last paragraph: nothing to read there
    run++;
    synth.cancel();
    readBlock(to);
  });
  addEventListener("pagehide", stop);
  // pause holds the voice mid-sentence (a finger held on the photo) and
  // resume carries on from there.
  function pause(on) { if (reading) { if (on) synth.pause(); else synth.resume(); } }
  S.narrator = { start: start, stop: stop, pause: pause, toggle: toggle, reading: function () { return reading; },
    blocks: function () { return blocks; },
    block: function () { return reading ? blocks[idx] : null; } };
})();
