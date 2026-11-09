/* ssg story — pronunciation rules for the narrator.
 * Text is written for the eye ("100g", "180°C", "np."); a voice reading it
 * says "sto g" or spells the symbol. `narrator.pronounce` maps what is
 * written to what is said:
 *
 *   pronounce:
 *     "{num} g":  "{num} gram|gramy|gramów|grama"
 *     "{num} łyżeczka|łyżeczki|łyżeczek": "{num:jedna|dwie} {word}"
 *     "np.":      "na przykład"
 *
 * In a pattern: {num} matches a number (100, 1,5, 0.25); a space means
 * "a space or none", so "{num} g" catches "100g" and "100 g"; a|b|c matches
 * any of those words. Patterns match whole words only: "{num} l" leaves
 * "5 lat" alone. The longest pattern wins.
 * In what is said:
 *   {num}          the number as written (the voice reads digits well)
 *   {num:one|two}  the number in words, for nouns whose gender the voice
 *                  gets wrong: 1 → "one", a final 2 → "two" (2, 22, 102),
 *                  so {num:jedna|dwie} reads 1 → jedna, 22 → dwadzieścia dwie;
 *                  Polish only (other languages and fractions keep digits)
 *   {word}         the word the pattern's a|b|c matched, as written
 *   a|b|c|d        the form that agrees with the number before it, in the
 *                  order of the language's plural categories — Polish: one (1),
 *                  few (2–4, 22…), many (5, 12, 25…), other (fractions: 1,5
 *                  grama). A form left out falls back to the last one given.
 * The text on screen and in subtitles is never changed — only what is spoken.
 * story_say.go does the same for the recorded Web Story narration;
 * testdata/story/pronounce.json holds the cases both must agree on. */
(function (root) {
  "use strict";
  var ORDER = ["zero", "one", "two", "few", "many", "other"];
  var NUM = "(\\d+(?:[.,]\\d+)?)";
  var WORD = /[\p{L}\p{N}]/u;
  var PL = {
    units: "zero jeden dwa trzy cztery pięć sześć siedem osiem dziewięć".split(" "),
    teens: "dziesięć jedenaście dwanaście trzynaście czternaście piętnaście szesnaście siedemnaście osiemnaście dziewiętnaście".split(" "),
    tens: ["", ""].concat("dwadzieścia trzydzieści czterdzieści pięćdziesiąt sześćdziesiąt siedemdziesiąt osiemdziesiąt dziewięćdziesiąt".split(" ")),
    hundreds: [""].concat("sto dwieście trzysta czterysta pięćset sześćset siedemset osiemset dziewięćset".split(" "))
  };

  function esc(s) { return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"); }

  // Polish words for 0–999; two replaces a final "dwa".
  function hundreds(n, two) {
    var out = [PL.hundreds[Math.floor(n / 100)]], r = n % 100;
    if (r >= 10 && r < 20) out.push(PL.teens[r - 10]);
    else {
      out.push(PL.tens[Math.floor(r / 10)]);
      if (r % 10 || !n) out.push(r % 10 === 2 && two ? two : PL.units[r % 10]);
    }
    return out.filter(Boolean).join(" ");
  }
  // The number in words, or null when there are none to give.
  function words(num, lang, one, two) {
    if (lang !== "pl" || !/^\d+$/.test(num) || num.length > 6) return null;
    var n = parseInt(num, 10);
    if (n === 1) return one;
    var th = Math.floor(n / 1000), rest = n % 1000, out = [];
    if (th) {
      var f = th % 10, t = th % 100;
      out.push(th === 1 ? "tysiąc" : hundreds(th) + " " +
        (f >= 2 && f <= 4 && (t < 12 || t > 14) ? "tysiące" : "tysięcy"));
    }
    if (rest || !th) out.push(hundreds(rest, two));
    return out.join(" ");
  }

  // One pattern to a regex source, and what each of its groups captures.
  function compile(key) {
    key = key.trim();
    var kinds = [];
    var body = key.split("{num}").map(function (part, i) {
      if (i) kinds.push("num");
      return part.split(/\s+/).map(function (w) {
        if (w.indexOf("|") < 0) return esc(w);
        kinds.push("word");
        return "(" + w.split("|").map(esc).join("|") + ")";
      }).join("\\s*");
    }).join(NUM);
    var head = key.indexOf("{num}") === 0 || WORD.test(key[0]) ? "(?<![\\p{L}\\p{N}])" : "";
    var tail = key.slice(-5) === "{num}" || WORD.test(key.slice(-1)) ? "(?![\\p{L}\\p{N}])" : "";
    return { src: head + body + tail, kinds: kinds };
  }

  function make(rules, lang) {
    lang = (lang || "en").toLowerCase().split(/[-_]/)[0];
    var keys = Object.keys(rules || {}).sort(function (a, b) { return b.length - a.length; });
    if (!keys.length) return function (t) { return t; };
    var plural = new Intl.PluralRules(lang);
    var cats = plural.resolvedOptions().pluralCategories.slice().sort(function (a, b) {
      return ORDER.indexOf(a) - ORDER.indexOf(b);
    });
    // Each key is a group of its own, so we know which rule matched.
    var groups = [], at = 1;
    var re = new RegExp(keys.map(function (k) {
      var c = compile(k);
      groups.push({ key: k, index: at, kinds: c.kinds });
      at += c.kinds.length + 1;
      return "(" + c.src + ")";
    }).join("|"), "gu");

    function form(word, num) {
      var forms = word.split("|");
      if (num == null) return forms[0];
      var i = cats.indexOf(plural.select(parseFloat(num.replace(",", "."))));
      return forms[Math.min(i < 0 ? forms.length - 1 : i, forms.length - 1)];
    }
    function say(g, m) {
      var nums = [], said = [], k = 0, last = null;
      g.kinds.forEach(function (kind, j) { (kind === "num" ? nums : said).push(m[g.index + 1 + j]); });
      return String(rules[g.key]).split(/(\s+)/).map(function (tok) {
        var out = tok.replace(/\{word\}/g, function () { return said[0] || ""; })
          .replace(/\{num(?::([^|}]*)\|([^}]*))?\}/g, function (_, one, two) {
            last = nums[k++] || "";
            return (one != null && words(last, lang, one, two)) || last;
          });
        return out.indexOf("|") >= 0 ? form(out, last) : out;
      }).join("");
    }
    return function (text) {
      return text.replace(re, function () {
        var m = Array.prototype.slice.call(arguments);
        for (var j = 0; j < groups.length; j++) if (m[groups[j].index] !== undefined) return say(groups[j], m);
        return m[0];
      });
    };
  }

  if (typeof module === "object" && module.exports) module.exports = make;
  var S = root && root.ssgStory;
  if (S) S.pronounce = make((S.cfg.narrator || {}).pronounce, document.documentElement.lang || "en");
})(typeof window === "undefined" ? null : window);
