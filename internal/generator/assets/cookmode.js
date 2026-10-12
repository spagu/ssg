// Cook Mode (1.8.72): a recipe one screen at a time, for the kitchen.
//
// Each button marked data-ssg-cook carries its recipe as JSON in
// data-recipe ({name, ingredients, steps}) and optional label overrides in
// data-labels. Pressing it opens a full-screen modal <dialog>: the
// ingredients as a checklist, one large-text screen per step, then a finish
// screen. Next/Back, the Left/Right arrow keys, a horizontal swipe, or a tap
// on the right/left half of the screen move between them; Escape closes and
// focus returns to the button. While it is open the screen is kept awake
// where the Screen Wake Lock API exists. The current screen and the ticked
// ingredients are kept in localStorage per page, so a reload — or a phone
// that locked anyway — comes back to the same step.
(function () {
  'use strict';

  var DEFAULTS = {
    title: 'Cook mode', ingredients: 'Ingredients', step: 'Step {n} of {total}', next: 'Next',
    back: 'Back', close: 'Close', restart: 'Start over', done: 'All done', finished: 'Enjoy your meal.'
  };

  // labelsWith is the default texts with the configured ones on top; only
  // non-empty strings replace a default.
  function labelsWith(over) {
    var out = {};
    Object.keys(DEFAULTS).forEach(function (k) {
      out[k] = over && typeof over[k] === 'string' && over[k].trim() ? over[k] : DEFAULTS[k];
    });
    return out;
  }

  // lastScreen is the index of the finish screen: ingredients, the steps, done.
  function lastScreen(recipe) {
    return recipe.steps.length + 1;
  }

  // clampScreen keeps a screen index inside the recipe.
  function clampScreen(i, recipe) {
    i = Math.floor(Number(i)) || 0;
    return Math.max(0, Math.min(lastScreen(recipe), i));
  }

  // progressText is what the live region says for screen i.
  function progressText(i, recipe, labels) {
    if (i <= 0) return labels.ingredients;
    if (i > recipe.steps.length) return labels.done;
    return labels.step.replace('{n}', String(i)).replace('{total}', String(recipe.steps.length));
  }

  // parseState reads saved progress, dropping anything that does not fit
  // the recipe as it is now (an edited recipe may have fewer steps).
  function parseState(raw, recipe) {
    var state = { screen: 0, checked: [] };
    try {
      var v = JSON.parse(raw);
      if (v && typeof v === 'object') {
        state.screen = clampScreen(v.screen, recipe);
        state.checked = (Array.isArray(v.checked) ? v.checked : []).filter(function (n, i, all) {
          return n === Math.floor(n) && n >= 0 && n < recipe.ingredients.length && all.indexOf(n) === i;
        });
      }
    } catch (e) { /* nothing saved, or not ours */ }
    return state;
  }

  // storageKey names a page's saved progress.
  function storageKey(path, index) {
    return 'ssg-cook:' + path + (index ? '#' + index : '');
  }

  // swipeDirection turns a finished touch movement into +1 (next, finger
  // moved left), -1 (back) or 0 (too short, or more vertical than not).
  function swipeDirection(dx, dy) {
    if (Math.abs(dx) < 50 || Math.abs(dx) < Math.abs(dy) * 1.5) return 0;
    return dx < 0 ? 1 : -1;
  }

  // tapDirection: the right half of the screen goes on, the left goes back.
  function tapDirection(x, width) {
    return x >= width / 2 ? 1 : -1;
  }

  if (typeof module === 'object' && module.exports) {
    module.exports = {
      labelsWith: labelsWith, lastScreen: lastScreen, clampScreen: clampScreen, progressText: progressText,
      parseState: parseState, storageKey: storageKey, swipeDirection: swipeDirection, tapDirection: tapDirection
    };
    return;
  }

  // Without a modal <dialog> the button stays hidden: a half-working
  // full-screen view is worse than the recipe on the page.
  if (typeof HTMLDialogElement !== 'function') return;

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function button(cls, text) {
    var b = el('button', cls, text);
    b.type = 'button';
    return b;
  }

  function setup(opener, index) {
    var recipe, labels;
    try {
      recipe = JSON.parse(opener.getAttribute('data-recipe'));
      labels = labelsWith(JSON.parse(opener.getAttribute('data-labels') || '{}'));
    } catch (e) {
      return;
    }
    if (!recipe || !Array.isArray(recipe.ingredients) || !Array.isArray(recipe.steps) || !recipe.steps.length) return;
    opener.hidden = false;

    var key = storageKey(location.pathname, index), state, dialog, stage, back, next, progress, lock = null;
    var id = 'ssg-cook-' + index;

    function load() {
      var raw = null;
      try { raw = localStorage.getItem(key); } catch (e) { /* storage blocked */ }
      state = parseState(raw, recipe);
    }

    function save() {
      try { localStorage.setItem(key, JSON.stringify(state)); } catch (e) { /* storage blocked or full */ }
    }

    function lockScreen() {
      if (!navigator.wakeLock || !dialog.open || lock) return;
      navigator.wakeLock.request('screen').then(function (l) {
        lock = l;
        l.addEventListener('release', function () { if (lock === l) lock = null; });
      }, function () { /* refused: low battery, or not visible */ });
    }

    function unlockScreen() {
      if (lock) {
        var l = lock;
        lock = null;
        l.release().catch(function () {});
      }
    }

    function renderScreen() {
      var screen = el('div', 'ssg-cook-screen'), i = state.screen;
      if (i === 0) {
        screen.appendChild(el('h3', 'ssg-cook-heading', labels.ingredients));
        var list = el('ul', 'ssg-cook-ingredients');
        recipe.ingredients.forEach(function (text, n) {
          var item = el('li'), label = el('label'), box = el('input');
          box.type = 'checkbox';
          box.checked = state.checked.indexOf(n) >= 0;
          box.addEventListener('change', function () {
            state.checked = state.checked.filter(function (c) { return c !== n; });
            if (box.checked) state.checked.push(n);
            save();
          });
          label.appendChild(box);
          label.appendChild(el('span', '', text));
          item.appendChild(label);
          list.appendChild(item);
        });
        screen.appendChild(list);
      } else if (i <= recipe.steps.length) {
        screen.appendChild(el('p', 'ssg-cook-step', recipe.steps[i - 1]));
      } else {
        screen.appendChild(el('h3', 'ssg-cook-heading', labels.done));
        screen.appendChild(el('p', 'ssg-cook-step', labels.finished));
      }
      stage.textContent = '';
      stage.appendChild(screen);
      var atEnd = i === lastScreen(recipe);
      back.disabled = i === 0;
      next.textContent = atEnd ? labels.close : labels.next;
      progress.textContent = progressText(i, recipe, labels);
      if (back.disabled && document.activeElement === back) next.focus();
    }

    function go(dir) {
      if (dir > 0 && state.screen === lastScreen(recipe)) {
        close();
        return;
      }
      var to = clampScreen(state.screen + dir, recipe);
      if (to === state.screen) return;
      state.screen = to;
      save();
      renderScreen();
    }

    function close() {
      unlockScreen();
      if (dialog.open) dialog.close();
      document.documentElement.classList.remove('ssg-cook-open');
      opener.focus();
    }

    function focusables() {
      return [].filter.call(dialog.querySelectorAll('button, input, a[href]'), function (e) { return !e.disabled; });
    }

    function build() {
      dialog = el('dialog', 'ssg-cook-dialog');
      dialog.setAttribute('aria-modal', 'true');
      dialog.setAttribute('aria-labelledby', id + '-title');
      var top = el('div', 'ssg-cook-top'), title = el('h2', 'ssg-cook-title', recipe.name || labels.title);
      title.id = id + '-title';
      var restart = button('ssg-cook-action', labels.restart), shut = button('ssg-cook-action', labels.close);
      top.appendChild(title);
      top.appendChild(restart);
      top.appendChild(shut);
      stage = el('div', 'ssg-cook-stage');
      var nav = el('div', 'ssg-cook-nav');
      back = button('ssg-cook-back', labels.back);
      next = button('ssg-cook-next', labels.next);
      progress = el('p', 'ssg-cook-progress');
      progress.setAttribute('aria-live', 'polite');
      progress.setAttribute('role', 'status');
      nav.appendChild(back);
      nav.appendChild(progress);
      nav.appendChild(next);
      dialog.appendChild(top);
      dialog.appendChild(stage);
      dialog.appendChild(nav);
      document.body.appendChild(dialog);

      back.addEventListener('click', function () { go(-1); });
      next.addEventListener('click', function () { go(1); });
      shut.addEventListener('click', close);
      restart.addEventListener('click', function () {
        state = { screen: 0, checked: [] };
        try { localStorage.removeItem(key); } catch (e) { /* storage blocked */ }
        renderScreen();
        next.focus();
      });
      // Escape: close our way, so the wake lock and focus are handled.
      dialog.addEventListener('cancel', function (e) {
        e.preventDefault();
        close();
      });
      dialog.addEventListener('keydown', function (e) {
        if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
          e.preventDefault();
          go(e.key === 'ArrowRight' ? 1 : -1);
        } else if (e.key === 'Tab') {
          var f = focusables(), first = f[0], last = f[f.length - 1];
          if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last.focus();
          } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      });
      stage.addEventListener('click', function (e) {
        if (e.target.closest('input, label, button, a')) return;
        var r = stage.getBoundingClientRect();
        go(tapDirection(e.clientX - r.left, r.width));
      });
      var startX = 0, startY = 0;
      stage.addEventListener('touchstart', function (e) {
        startX = e.changedTouches[0].clientX;
        startY = e.changedTouches[0].clientY;
      }, { passive: true });
      stage.addEventListener('touchend', function (e) {
        var dir = swipeDirection(e.changedTouches[0].clientX - startX, e.changedTouches[0].clientY - startY);
        if (dir) {
          e.preventDefault(); // no tap after a swipe
          go(dir);
        }
      });
      // The browser drops the lock when the tab is hidden; take it back.
      document.addEventListener('visibilitychange', function () {
        if (document.visibilityState === 'visible') lockScreen();
        else lock = null;
      });
    }

    opener.addEventListener('click', function () {
      if (!dialog) build();
      load();
      document.documentElement.classList.add('ssg-cook-open');
      dialog.showModal();
      renderScreen();
      next.focus();
      lockScreen();
    });
  }

  document.querySelectorAll('button[data-ssg-cook]').forEach(setup);
})();
