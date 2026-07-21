/* ssgtheme — progressive enhancement only.
 *
 * Nothing here is required to read the site: the navigation is a plain wrapping
 * list (it reflows to a column under 768 px, exactly like the design system's
 * own header), so there is no menu to open and no JavaScript to depend on.
 * Two behaviours are added when it runs: the colour-scheme switch and marking
 * the current navigation entry. */

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

  /** 1234 → "1.2k", matching how repository badges read. */
  function formatCount(n) {
    if (n < 1000) { return String(n); }
    return (Math.round(n / 100) / 10).toFixed(1).replace(/\.0$/, '') + 'k';
  }

  function init() {
    initScheme();
    markCurrentPage();
    initStars();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
