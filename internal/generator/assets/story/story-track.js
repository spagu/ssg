/* ssg story — analytics and lock-screen controls.
 * Analytics: every story event goes to the page's dataLayer, so Google Tag
 * Manager (or anything reading dataLayer) decides where it is sent and
 * whether consent allows it — the story itself sends nothing anywhere.
 * Events: story_open, story_slide, story_complete, story_close (with seconds
 * spent), story_narrator, story_sound, story_share, story_ad_click.
 * `analytics: false` turns it off, `analytics.prefix` renames the events.
 * Lock screen: while music plays, the phone's lock screen, notification and
 * headset buttons show the article and can pause, go back and forward
 * (Media Session API). Browsers show these only for real audio, so speech
 * alone does not appear there. Loaded after story-show.js. */
(function () {
  "use strict";
  var S = window.ssgStory;
  if (!S) return;
  var h1 = document.querySelector("h1");
  var name = h1 ? h1.textContent.trim() : document.title;

  // ---- analytics ---------------------------------------------------------------
  var a = S.cfg.analytics === false ? null : Object.assign({ prefix: "story_" }, S.cfg.analytics);
  var opened = 0;
  function push(type, data) {
    if (!a) return;
    var dl = window.dataLayer = window.dataLayer || [];
    dl.push(Object.assign({ event: a.prefix + type, story_title: name, story_path: location.pathname }, data));
  }
  document.addEventListener("ssg-story", function (e) {
    var d = e.detail;
    switch (d.type) {
      case "open": opened = Date.now(); push("open", { story_slides: d.total }); break;
      case "slide": push("slide", { story_slide: d.index + 1, story_slides: d.total, story_slide_kind: d.kind, story_slide_label: d.label }); break;
      case "complete": push("complete", { story_slides: d.total }); break;
      case "close": push("close", { story_slide: d.index + 1, story_slides: d.total, story_seconds: opened ? Math.round((Date.now() - opened) / 1000) : 0 }); break;
      case "narrator": push("narrator", { story_on: d.on }); break;
      case "sound": push("sound", { story_on: d.on }); break;
      case "share": push("share", { story_method: d.method }); break;
    }
  });
  // A click on the sponsor slot, wherever it sits.
  document.addEventListener("click", function (e) {
    var link = e.target.closest && e.target.closest(".story-ad a");
    if (link) push("ad_click", { story_link: link.href });
  }, true);

  // ---- lock-screen controls ------------------------------------------------------
  var ms = navigator.mediaSession;
  if (!ms || !S.audio) return;
  var og = document.querySelector('meta[property="og:image"]');
  var site = document.querySelector('meta[property="og:site_name"]');
  function metadata() {
    ms.metadata = new MediaMetadata({
      title: name,
      artist: site ? site.content : location.hostname,
      artwork: og && og.content ? [{ src: og.content, sizes: "1200x630" }] : []
    });
  }
  function set(action, fn) { try { ms.setActionHandler(action, fn); } catch (err) { /* not supported here */ } }
  set("play", function () { S.sound(true); if (S.show.open()) S.show.play(true); });
  set("pause", function () { S.sound(false); if (S.narrator) S.narrator.stop(); if (S.show.open()) S.show.play(false); });
  set("previoustrack", function () { if (S.show.open()) S.show.go(S.show.at() - 1); });
  set("nexttrack", function () { if (S.show.open()) S.show.go(S.show.at() + 1); });
  document.addEventListener("ssg-story", function (e) {
    if (e.detail.type === "sound") { if (e.detail.on) metadata(); ms.playbackState = e.detail.on ? "playing" : "paused"; }
  });
})();
