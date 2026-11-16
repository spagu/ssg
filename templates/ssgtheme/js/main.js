/* Cache-bust marker. The edge pinned a 404 page under this file's previous
   content-addressed URL with a year-long immutable header, which no redeploy
   could correct; changing the bytes changes the URL. Last bust: 2026-08-07. */

/* ssgtheme — progressive enhancement only.
 *
 * Nothing here is required to read the site: the navigation is a plain wrapping
 * list (it reflows to a column under 768 px, exactly like the design system's
 * own header), so there is no menu to open and no JavaScript to depend on.
 * What it adds when it runs: the colour-scheme switch, marking the current
 * navigation entry, the folding docs sidebar on narrow screens, and the
 * GitHub star count. */

(function () {
  'use strict';

  var SCHEME_KEY = 'ssgtheme-scheme';

  /** Colour scheme: an explicit choice is stored and beats the OS preference. */
  function initScheme() {
    var toggle = document.getElementById('theme-toggle');
    if (!toggle) { return; }

    var root = document.documentElement;
    var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;

    function current() {
      return root.getAttribute('data-theme') || (prefersDark ? 'dark' : 'light');
    }

    function apply(scheme) {
      root.setAttribute('data-theme', scheme);
      toggle.setAttribute('aria-pressed', String(scheme === 'dark'));
      try { localStorage.setItem(SCHEME_KEY, scheme); } catch (e) { /* private mode */ }
    }

    apply(current());
    toggle.addEventListener('click', function () {
      apply(current() === 'dark' ? 'light' : 'dark');
    });
  }

  /** Marks the navigation entry matching the current path, for CSS and AT. */
  function markCurrentPage() {
    var here = window.location.pathname.replace(/\/+$/, '') || '/';
    var links = document.querySelectorAll('#nav-list a');
    for (var i = 0; i < links.length; i++) {
      var path = links[i].getAttribute('href').replace(/\/+$/, '') || '/';
      if (path === here) { links[i].setAttribute('aria-current', 'page'); }
    }
  }

  /** GitHub star count, filled in only if the API answers. The element starts
   *  hidden so a rate-limited or offline reader never sees an empty badge. */
  function initStars() {
    var el = document.getElementById('gh-stars');
    var count = document.getElementById('gh-stars-count');
    if (!el || !count || !window.fetch) { return; }

    var repo = el.getAttribute('data-repo');
    if (!repo) { return; }

    fetch('https://api.github.com/repos/' + repo, { headers: { Accept: 'application/vnd.github+json' } })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) {
        // A repository with no stars yet gets no badge — an empty "★" reads
        // as a broken widget, not as information.
        if (!data || typeof data.stargazers_count !== 'number' || data.stargazers_count < 1) { return; }
        count.textContent = formatCount(data.stargazers_count);
        el.hidden = false;
      })
      .catch(function () { /* offline or rate-limited: leave it hidden */ });
  }

  /** Docs navigation on a narrow screen. Below the two-column breakpoint the
   *  CSS folds the guide list behind a sticky "Documentation / <page>" bar
   *  (only under .sc-js, set in <head> before first paint); this opens it as
   *  an overlay. While open the rest of the page is inert, so focus stays in
   *  the list; Escape, a tap outside or picking a guide closes it, and focus
   *  returns to the bar. */
  function initDocsNav() {
    var nav = document.querySelector('.docs-nav--fold');
    var toggle = nav && nav.querySelector('.docs-nav__toggle');
    var panel = toggle && document.getElementById(toggle.getAttribute('aria-controls'));
    if (!panel) { return; }

    var root = document.documentElement;
    var header = document.querySelector('.site-header');
    var narrow = window.matchMedia('(max-width: 999.98px)');
    var inerted = [];

    // The bar sticks just below the header. The header's height depends on how
    // its links wrap, and on a long page it eventually scrolls away (it is
    // sticky only within the first screen), so the bar follows the header's
    // visible bottom edge rather than a fixed offset.
    var queued = false;
    function syncHeaderEdge() {
      queued = false;
      var edge = header ? Math.max(0, Math.round(header.getBoundingClientRect().bottom)) : 0;
      root.style.setProperty('--sc-header-h', edge + 'px');
    }
    function queueSync() {
      if (!queued) { queued = true; window.requestAnimationFrame(syncHeaderEdge); }
    }
    syncHeaderEdge();
    window.addEventListener('scroll', queueSync, { passive: true });
    window.addEventListener('resize', queueSync);

    function isOpen() { return nav.classList.contains('is-open'); }

    // Everything outside the nav, ancestor by ancestor, becomes inert while the
    // overlay is open — then restored exactly as it was.
    function setBackgroundInert(on) {
      if (!on) {
        for (var j = 0; j < inerted.length; j++) { inerted[j].inert = false; }
        inerted = [];
        return;
      }
      for (var el = nav; el && el.parentElement && el !== document.body; el = el.parentElement) {
        var siblings = el.parentElement.children;
        for (var i = 0; i < siblings.length; i++) {
          var s = siblings[i];
          if (s !== el && !s.inert && s.tagName !== 'SCRIPT') { s.inert = true; inerted.push(s); }
        }
      }
    }

    function open() {
      nav.classList.add('is-open');
      toggle.setAttribute('aria-expanded', 'true');
      setBackgroundInert(true);
      var target = panel.querySelector('[aria-current="page"]') || panel.querySelector('a');
      if (target) {
        target.focus({ preventScroll: true });
        // Show the current guide a third of the way down, with its group title
        // and neighbours, rather than at the very bottom edge. The panel is
        // positioned, so it is the link's offsetParent.
        panel.scrollTop = Math.max(0, target.offsetTop - panel.clientHeight / 3);
      }
    }

    function close(returnFocus) {
      if (!isOpen()) { return; }
      nav.classList.remove('is-open');
      toggle.setAttribute('aria-expanded', 'false');
      setBackgroundInert(false);
      if (returnFocus) { toggle.focus(); }
    }

    toggle.addEventListener('click', function () {
      if (isOpen()) { close(true); } else { open(); }
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && isOpen()) { close(true); }
    });
    document.addEventListener('click', function (e) {
      if (!isOpen()) { return; }
      if (!nav.contains(e.target) || e.target.classList.contains('docs-nav__scrim')) { close(false); }
    });
    panel.addEventListener('click', function (e) {
      if (e.target.closest && e.target.closest('a')) { close(false); }
    });
    // Widening the window past the breakpoint shows the list as a column again.
    var onChange = function () { if (!narrow.matches) { close(false); } };
    if (narrow.addEventListener) { narrow.addEventListener('change', onChange); } else { narrow.addListener(onChange); }
  }

  /** 1234 → "1.2k", matching how repository badges read. */
  function formatCount(n) {
    if (n < 1000) { return String(n); }
    return (Math.round(n / 100) / 10).toFixed(1).replace(/\.0$/, '') + 'k';
  }

  function init() {
    // Tells the <head> script this file ran, so it keeps .sc-js (see chrome.html).
    document.documentElement.classList.add('sc-ready');
    initScheme();
    markCurrentPage();
    initDocsNav();
    initStars();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
