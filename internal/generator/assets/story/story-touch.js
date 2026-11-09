/* ssg story — gestures in the presentation.
 * The photo answers the way stories do on a phone: a tap on the left third
 * goes back, a tap anywhere else goes on, a finger held down pauses (the
 * slide timer and the narrator) until it lifts, a swipe sideways changes the
 * slide and a swipe down closes the show. A mouse gets the same, so the
 * desktop behaves like the phone. Turn off with `slideshow.gestures: false`.
 * Loaded after story-show.js, before the show is first built. */
(function () {
  "use strict";
  var S = window.ssgStory;
  if (!S || !S.show || (S.cfg.slideshow || {}).gestures === false) return;
  S.touch = true; // story-show.js leaves clicks on the photo to us
  var HOLD = 350, SWIPE = 60, BACK = 1 / 3;
  var start = null, held = false, holdTimer = 0, wasPlaying = true;

  function hold(on) {
    held = on;
    if (on) { wasPlaying = S.show.playing(); S.show.play(false); } else S.show.play(wasPlaying);
    if (S.narrator) S.narrator.pause(on);
    var box = S.show.box();
    if (box) box.classList.toggle("is-held", on);
  }

  function down(e) {
    if (e.button > 0 || e.target.closest("a, button, input, select, label, .story-recipe")) return;
    start = { x: e.clientX, y: e.clientY, t: Date.now() };
    held = false;
    clearTimeout(holdTimer);
    holdTimer = setTimeout(function () { if (start) hold(true); }, HOLD);
  }
  function up(e) {
    if (!start) return;
    clearTimeout(holdTimer);
    var dx = e.clientX - start.x, dy = e.clientY - start.y, at = S.show.at();
    var wasHeld = held;
    start = null;
    if (wasHeld) { hold(false); return; } // a hold is a pause, not a tap
    if (dy > SWIPE && dy > Math.abs(dx)) { S.show.close(); return; }
    if (Math.abs(dx) > SWIPE) { S.show.go(dx < 0 ? at + 1 : at - 1); return; }
    if (Math.abs(dx) < 10 && Math.abs(dy) < 10) {
      var r = S.show.box().getBoundingClientRect();
      S.show.go(e.clientX - r.left < r.width * BACK ? at - 1 : at + 1);
    }
  }
  function cancel() {
    clearTimeout(holdTimer);
    if (held) hold(false);
    start = null;
  }

  // The show is built on first open; bind once it exists.
  var bound = false;
  document.addEventListener("ssg-story", function (e) {
    if (e.detail.type !== "open" || bound) return;
    var box = S.show.box();
    if (!box) return;
    bound = true;
    box.style.touchAction = "none"; // swipes are ours, not the page's scroll
    box.addEventListener("pointerdown", down);
    box.addEventListener("pointerup", up);
    box.addEventListener("pointercancel", cancel);
    box.addEventListener("pointerleave", cancel);
    box.addEventListener("contextmenu", function (ev) { if (held) ev.preventDefault(); }); // long-press menu on phones
  });
})();
