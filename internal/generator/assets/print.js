// Print / Save as PDF (1.8.72). Shows the print buttons a page carries
// (they stay hidden without JavaScript), opens the browser's print dialog —
// which offers "Save as PDF" — and writes the page address and today's date
// into the print-only footer line. "Print recipe" marks <html> with
// ssg-print-recipe for the length of that one print, so the print
// stylesheet shows only the recipe card.
(function () {
  'use strict';

  // cleanURL is the address without its #fragment: the page, not the place
  // on it the reader had scrolled to.
  function cleanURL(href) {
    var i = String(href).indexOf('#');
    return i < 0 ? String(href) : String(href).slice(0, i);
  }

  // formatDate writes a date the way the page's language does, falling back
  // to ISO when the language is unknown to the runtime.
  function formatDate(date, lang) {
    try {
      return date.toLocaleDateString(lang || undefined, { year: 'numeric', month: 'long', day: 'numeric' });
    } catch (e) {
      return date.toISOString().slice(0, 10);
    }
  }

  // footerText is the print footer line.
  function footerText(url, printed, date) {
    return cleanURL(url) + ' — ' + printed + ' ' + date;
  }

  if (typeof module === 'object' && module.exports) {
    module.exports = { cleanURL: cleanURL, formatDate: formatDate, footerText: footerText };
    return;
  }

  var root = document.documentElement, recipeMode = false;
  var footer = document.querySelector('[data-ssg-print-footer]');

  function prepare() {
    if (footer) {
      footer.textContent = footerText(location.href, footer.getAttribute('data-printed') || 'Printed',
        formatDate(new Date(), root.lang));
    }
    root.classList.toggle('ssg-print-recipe', recipeMode);
  }

  // A print started from the browser menu prints the article; only the
  // recipe button turns on the card, and only until that print is done.
  window.addEventListener('beforeprint', prepare);
  window.addEventListener('afterprint', function () {
    recipeMode = false;
    root.classList.remove('ssg-print-recipe');
  });

  document.querySelectorAll('[data-ssg-print]').forEach(function (box) {
    box.hidden = false;
    box.querySelectorAll('button[data-print]').forEach(function (b) {
      b.hidden = false;
      b.addEventListener('click', function () {
        recipeMode = b.getAttribute('data-print') === 'recipe';
        prepare();
        window.print();
      });
    });
  });
})();
