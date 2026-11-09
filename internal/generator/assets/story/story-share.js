/* ssg story — sharing and links into the presentation.
 * A Share button next to Listen: on a phone it opens the system share sheet
 * (Facebook, Instagram, WhatsApp… — the reader picks post or story there,
 * no page can pick for them); elsewhere a small menu of networks, Copy link
 * and, when the build made them, the 9:16 picture of the slide for a story.
 * In the presentation the shared link points at the slide on screen:
 * `#story` opens the show at its title, `#story=3` at its third slide, and
 * the address follows the show while it plays. Loaded after story-show.js. */
(function () {
  "use strict";
  var S = window.ssgStory;
  if (!S || !S.show) return;
  var cfg = S.cfg.share === false ? null : Object.assign({
    networks: ["facebook", "whatsapp", "x", "pinterest", "email", "copy"], native: "touch" }, S.cfg.share);
  var L = Object.assign({ share: "Share", copy: "Copy link", copied: "Copied", storyImage: "Picture for a story (9:16)", webStory: "Web Story",
    start: "Play with narrator" }, S.cfg.labels);
  var HASH = /^#story(?:=(\d+))?$/;

  // ---- the address follows the show ----------------------------------------
  function base() {
    var c = document.querySelector('link[rel="canonical"]');
    return (c && c.href) || location.href.split("#")[0];
  }
  function link(inShow) {
    if (!inShow || !S.show.open()) return base();
    var at = S.show.at();
    return base() + (at > 0 ? "#story=" + (at + 1) : "#story");
  }
  document.addEventListener("ssg-story", function (e) {
    // A full address: a page with <base href> would resolve a bare path
    // against another origin, which replaceState refuses.
    var d = e.detail, path = location.origin + location.pathname + location.search;
    if (d.type === "slide" && S.show.open()) history.replaceState(null, "", path + (d.index > 0 ? "#story=" + (d.index + 1) : "#story"));
    if (d.type === "close") history.replaceState(null, "", path);
  });

  // open follows a #story link. The browser lets a page go fullscreen and
  // speak only after the visitor touches it, so the show opens quietly with
  // a Play button that starts the narrator, music and fullscreen.
  function open() {
    var m = HASH.exec(location.hash);
    if (!m || !window.ssgStoryShow) return;
    if (!S.show.open()) window.ssgStoryShow(0, true);
    var n = m[1] ? parseInt(m[1], 10) - 1 : 0;
    if (n > 0) S.show.go(Math.min(n, S.show.total() - 1));
    var box = S.show.box();
    if (!box || box.querySelector(".story-start")) return;
    var b = S.button(L.start, "M8 5v14l11-7z", function () {
      b.remove();
      var show = box.closest(".story-show");
      if (show.requestFullscreen && !document.fullscreenElement) show.requestFullscreen().catch(function () {});
      if (S.audio && !S.soundOn() && (S.cfg.audio || {}).withPresentation !== false) S.sound(true);
      if (S.narrator && (S.cfg.slideshow || {}).narrate !== false) S.narrator.start({ fromTop: S.show.at() === 0 });
    });
    b.classList.add("story-start");
    box.appendChild(b);
    b.focus({ preventScroll: true });
  }
  addEventListener("hashchange", open);
  if (HASH.test(location.hash)) {
    if (document.readyState === "complete") open(); else addEventListener("load", open);
  }

  if (!cfg) return;

  // ---- the Share button --------------------------------------------------------
  var NETWORKS = {
    facebook: ["Facebook", "https://www.facebook.com/sharer/sharer.php?u={url}"],
    x: ["X", "https://x.com/intent/post?url={url}&text={text}"],
    whatsapp: ["WhatsApp", "https://wa.me/?text={text}%20{url}"],
    telegram: ["Telegram", "https://t.me/share/url?url={url}&text={text}"],
    linkedin: ["LinkedIn", "https://www.linkedin.com/sharing/share-offsite/?url={url}"],
    pinterest: ["Pinterest", "https://pinterest.com/pin/create/button/?url={url}&media={image}&description={text}"],
    email: ["E-mail", "mailto:?subject={text}&body={url}"]
  };
  function title() {
    var h1 = document.querySelector("h1");
    return cfg.text || (h1 ? h1.textContent.trim() : document.title);
  }
  function image() {
    var og = document.querySelector('meta[property="og:image"]');
    return og ? og.content : "";
  }
  function fill(tpl, inShow) {
    return tpl.replace("{url}", encodeURIComponent(link(inShow)))
      .replace("{text}", encodeURIComponent(title()))
      .replace("{image}", encodeURIComponent(image()));
  }
  // storyImage is the 9:16 picture of the slide on screen (or of the title
  // card while reading), made at build time; null when there are none.
  function storyImage(inShow) {
    var list = cfg.storyImages || [];
    if (!list.length) return null;
    var at = inShow && S.show.open() ? Math.max(0, S.show.at()) : 0;
    return list[Math.min(at, list.length - 1)];
  }
  function useNative() {
    if (!navigator.share || cfg.native === false) return false;
    return cfg.native === true || matchMedia("(pointer: coarse)").matches;
  }

  function make(where) {
    var inShow = where === "show";
    var group = document.createElement("span");
    group.className = "story-share";
    var menu = document.createElement("div");
    menu.className = "story-share-menu";
    menu.hidden = true;
    menu.setAttribute("role", "menu");
    function item(label, href, method) {
      var a = document.createElement("a");
      a.setAttribute("role", "menuitem");
      a.textContent = label;
      a.href = href;
      if (/^https?:/.test(href)) { a.target = "_blank"; a.rel = "noopener"; }
      a.addEventListener("click", function () { S.emit("share", { method: method }); menu.hidden = true; });
      menu.appendChild(a);
      return a;
    }
    function build() { // rebuilt on each open: the slide on screen changes the link
      menu.innerHTML = "";
      cfg.networks.forEach(function (n) {
        if (n === "copy") {
          var c = item(L.copy, "#", "copy");
          c.addEventListener("click", function (e) {
            e.preventDefault();
            var done = function () { c.textContent = L.copied; setTimeout(function () { menu.hidden = true; }, 900); };
            if (navigator.clipboard) navigator.clipboard.writeText(link(inShow)).then(done, function () {});
          });
        } else if (NETWORKS[n]) item(NETWORKS[n][0], fill(NETWORKS[n][1], inShow), n);
      });
      var img = storyImage(inShow);
      if (img) item(L.storyImage, img, "story-image").setAttribute("download", "");
      if (cfg.webStory) item(L.webStory, cfg.webStory, "web-story"); // the same slides as an AMP Web Story
    }
    var btn = S.button(L.share, "M18 16a3 3 0 0 0-2.4 1.2l-6.7-3.4a3 3 0 0 0 0-1.6l6.7-3.4A3 3 0 1 0 15 7l-6.7 3.4a3 3 0 1 0 0 3.2L15 17a3 3 0 1 0 3-1z", function () {
      if (useNative()) {
        navigator.share({ title: title(), url: link(inShow) })
          .then(function () { S.emit("share", { method: "native" }); }, function () { /* the reader closed the sheet */ });
        return;
      }
      if (menu.hidden) build();
      menu.hidden = !menu.hidden;
      btn.setAttribute("aria-expanded", menu.hidden ? "false" : "true");
    });
    btn.setAttribute("aria-haspopup", "menu");
    btn.setAttribute("aria-expanded", "false");
    group.addEventListener("keydown", function (e) { if (e.key === "Escape" && !menu.hidden) { menu.hidden = true; btn.focus(); e.stopPropagation(); } });
    document.addEventListener("click", function (e) { if (!group.contains(e.target)) menu.hidden = true; });
    group.appendChild(menu);
    group.appendChild(btn);
    return group;
  }

  S.controls.insertBefore(make("reading"), S.controls.firstChild);
  (S.extraButtons = S.extraButtons || []).push(make);
})();
