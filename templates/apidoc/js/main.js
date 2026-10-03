// apidoc — the behaviour the theme adds on top of plain HTML. Each part
// works on its own and fails quietly: without JavaScript the sidebar is
// reachable from the home page, the table of contents stays hidden, and the
// search box is a text field.
(function () {
  'use strict';

  // Colour scheme: the button cycles light/dark and remembers the choice.
  var scheme = document.getElementById('ad-scheme');
  if (scheme) {
    var root = document.documentElement;
    var dark = function () {
      return root.dataset.theme ? root.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
    };
    scheme.setAttribute('aria-pressed', dark() ? 'true' : 'false');
    scheme.addEventListener('click', function () {
      var next = dark() ? 'light' : 'dark';
      root.dataset.theme = next;
      scheme.setAttribute('aria-pressed', next === 'dark' ? 'true' : 'false');
      try { localStorage.setItem('apidoc-scheme', next); } catch (e) { /* not remembered */ }
    });
  }

  // Menu: the sidebar as a drawer on narrow screens; Escape closes it.
  var menu = document.querySelector('.ad-menu'), sidebar = document.getElementById('ad-sidebar');
  if (menu && sidebar) {
    var setOpen = function (open) {
      sidebar.classList.toggle('is-open', open);
      menu.setAttribute('aria-expanded', open ? 'true' : 'false');
    };
    menu.addEventListener('click', function () { setOpen(!sidebar.classList.contains('is-open')); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && sidebar.classList.contains('is-open')) { setOpen(false); menu.focus(); }
    });
    var current = sidebar.querySelector('[aria-current="page"]');
    if (current && current.scrollIntoView) current.scrollIntoView({ block: 'center' });
  }

  // On this page: the article's h2 and h3 with ids, the one in view marked.
  var toc = document.querySelector('.ad-toc');
  var heads = Array.prototype.slice.call(document.querySelectorAll('.ad-prose h2[id], .ad-prose h3[id]'));
  if (toc && heads.length > 1) {
    var list = toc.querySelector('ul'), links = {};
    heads.forEach(function (h) {
      var li = document.createElement('li'), a = document.createElement('a');
      if (h.tagName === 'H3') li.className = 'is-sub';
      a.href = '#' + h.id;
      a.textContent = h.textContent;
      li.appendChild(a);
      list.appendChild(li);
      links[h.id] = a;
    });
    toc.hidden = false;
    if ('IntersectionObserver' in window) {
      var active = null;
      var observer = new IntersectionObserver(function (entries) {
        entries.forEach(function (en) {
          if (!en.isIntersecting) return;
          if (active) active.classList.remove('is-active');
          active = links[en.target.id];
          active.classList.add('is-active');
        });
      }, { rootMargin: '0px 0px -70% 0px' });
      heads.forEach(function (h) { observer.observe(h); });
    }
  }

  // Search over search-index.json, loaded on first use. Titles rank above
  // text; "/" focuses the box, arrows move through the results.
  var input = document.getElementById('ad-search-input');
  var results = document.getElementById('ad-search-results');
  var status = document.getElementById('ad-search-status');
  if (!input || !results) return;
  var docs = null, loading = null;
  function load() {
    if (!loading) {
      loading = fetch(input.getAttribute('data-index')).then(function (r) {
        if (!r.ok) throw new Error(r.status);
        return r.json();
      }).then(function (d) { docs = d || []; }).catch(function () { docs = []; status.textContent = 'Search is not available on this site.'; });
    }
    return loading;
  }
  function render(q) {
    results.textContent = '';
    q = q.trim().toLowerCase();
    if (q.length < 2) { results.hidden = true; status.textContent = ''; return; }
    var hits = [];
    docs.forEach(function (d) {
      var t = (d.title || '').toLowerCase(), score = t === q ? 3 : t.indexOf(q) >= 0 ? 2 : (d.text || '').toLowerCase().indexOf(q) >= 0 ? 1 : 0;
      if (score) hits.push([score, d]);
    });
    hits.sort(function (a, b) { return b[0] - a[0] || a[1].title.length - b[1].title.length; });
    hits.slice(0, 12).forEach(function (h) {
      var li = document.createElement('li'), a = document.createElement('a'), small = document.createElement('small');
      a.href = h[1].url;
      a.textContent = h[1].title;
      small.textContent = h[1].excerpt || '';
      a.appendChild(small);
      li.appendChild(a);
      results.appendChild(li);
    });
    results.hidden = hits.length === 0;
    status.textContent = hits.length ? hits.length + ' results' : 'No results';
  }
  input.addEventListener('focus', load);
  input.addEventListener('input', function () { load().then(function () { render(input.value); }); });
  input.addEventListener('keydown', function (e) {
    var items = results.querySelectorAll('a');
    if (e.key === 'ArrowDown' && items.length) { e.preventDefault(); items[0].focus(); }
    if (e.key === 'Escape') { input.value = ''; render(''); }
  });
  results.addEventListener('keydown', function (e) {
    var items = Array.prototype.slice.call(results.querySelectorAll('a')), i = items.indexOf(document.activeElement);
    if (e.key === 'ArrowDown' && i < items.length - 1) { e.preventDefault(); items[i + 1].focus(); }
    if (e.key === 'ArrowUp') { e.preventDefault(); (i > 0 ? items[i - 1] : input).focus(); }
    if (e.key === 'Escape') { input.focus(); render(''); input.value = ''; }
  });
  document.addEventListener('keydown', function (e) {
    var t = e.target.tagName;
    if (e.key === '/' && t !== 'INPUT' && t !== 'TEXTAREA' && t !== 'SELECT' && !e.target.isContentEditable) {
      e.preventDefault();
      input.focus();
    }
  });
  document.addEventListener('click', function (e) {
    if (!e.target.closest('.ad-search')) results.hidden = true;
  });
})();
