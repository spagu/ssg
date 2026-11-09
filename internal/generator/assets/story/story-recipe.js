/* ssg story — recipe widget.
 * `type: recipe` pins the ingredients next to the text for the whole read:
 * grouped, with boxes to tick off while cooking (remembered per page in this
 * browser), and the step the reader has reached ("krok 7 z 20").
 * Ingredients come from YAML `groups:`, or are read from the page itself
 * (`source: "text:Składniki:"`): a <p><strong>Group:</strong></p> starts a
 * group, the <ul>/<ol> after it holds the items, the first numbered step ends it.
 * Loaded before story.js, which renders widgets through window.ssgStoryWidgets. */
(function () {
  "use strict";
  var reg = window.ssgStoryWidgets = window.ssgStoryWidgets || {};

  // lines splits a block into its <br>-separated lines, so an old post that
  // writes the whole ingredient list as one paragraph reads like a list.
  // A line that opens with <strong> is a heading ("Ciasto – …", "Lukier:").
  function lines(block) {
    return block.innerHTML.split(/<br\s*\/?>/i).map(function (html) {
      var d = document.createElement("div");
      d.innerHTML = html;
      var text = d.textContent.replace(/\s+/g, " ").trim();
      var first = d.firstElementChild;
      var lead = d.firstChild && d.firstChild.nodeType === 3 && d.firstChild.textContent.trim() ? null : first;
      return { text: text, strong: lead && lead.tagName === "STRONG" ? lead.textContent.replace(/[:\s]+$/, "").trim() : "" };
    }).filter(function (l) { return l.text; });
  }
  function heading(title, rest) {
    return { title: title, note: rest.replace(/^[\s–—:-]+/, "").replace(/:$/, "").trim(), items: [] };
  }

  // fromPage reads the ingredients after the "Składniki" marker, in either
  // shape: lines in the marker's own paragraph (older posts), or sibling
  // <p><strong>Group:</strong></p> + <ul> blocks (newer ones). It stops at
  // the first numbered step.
  function fromPage(start, stepRe, marker) {
    var groups = [], cur = null, done = false;
    function line(l) {
      if (stepRe.test(l.text)) { done = true; return; }
      if (l.strong) { cur = heading(l.strong, l.text.slice(l.strong.length)); groups.push(cur); return; }
      if (!cur) { cur = heading("", ""); groups.push(cur); }
      cur.items.push(l.text);
    }
    if (!start) return groups;
    var own = lines(start), at = -1;
    own.forEach(function (l, i) { if (at < 0 && l.text.toLowerCase().indexOf(marker) !== -1) at = i; });
    own.slice(at + 1).forEach(function (l) { if (!done) line(l); });
    for (var n = start.nextElementSibling; n && !done; n = n.nextElementSibling) {
      if (/^H[1-4]$/.test(n.tagName)) break;
      if (/^[UO]L$/.test(n.tagName)) {
        if (!cur) { cur = heading("", ""); groups.push(cur); }
        n.querySelectorAll("li").forEach(function (li) { cur.items.push(li.textContent.trim()); });
      } else if (n.querySelector && n.querySelector("img") && !n.textContent.trim()) {
        continue; // a photo between groups
      } else if (n.tagName === "P") {
        var ls = lines(n);
        if (ls.length === 1 && ls[0].strong && !stepRe.test(ls[0].text)) line(ls[0]);
        else break;
      } else break;
    }
    return groups.filter(function (g) { return g.items.length; }); // a bold step heading is not a group
  }

  // stepOf returns the step number a block starts, or 0. The number may come
  // after a bold heading line ("Masa budyniowa:<br>1. Do rondelka…").
  function stepOf(block, stepRe) {
    var ls = lines(block);
    for (var i = 0; i < ls.length; i++) {
      if (stepRe.test(ls[i].text)) return parseInt(ls[i].text, 10) || 0;
      if (!ls[i].strong) return 0;
    }
    return 0;
  }

  reg.recipe = function (w, el, api) {
    var stepRe = new RegExp(w.steps || "^\\d+\\.\\s");
    var source = w.source || "text:Składniki";
    var marker = source.replace(/^text:/, "").replace(/:$/, "").toLowerCase();
    var groups = w.groups || fromPage(api.find(source), stepRe, marker);
    var steps = Array.prototype.filter.call(api.content.querySelectorAll("p, li"), function (p) {
      return stepOf(p, stepRe) > 0;
    });
    var L = Object.assign({ title: "Przepis", step: "krok {n} z {all}", show: "Pokaż", hide: "Zwiń" }, w.labels);
    var key = "ssg-story-recipe:" + location.pathname;
    var ticked = {};
    try { ticked = JSON.parse(localStorage.getItem(key)) || {}; } catch (e) { ticked = {}; }

    if (!groups.length) { el.remove(); return null; } // not a recipe post: no widget
    el.classList.add("story-recipe");
    var head = document.createElement("div");
    head.className = "story-recipe-head";
    var h = document.createElement("h3");
    h.textContent = w.title || L.title;
    var toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "story-recipe-toggle";
    head.appendChild(h);
    head.appendChild(toggle);
    var stepEl = document.createElement("p");
    stepEl.className = "story-recipe-step";
    var body = document.createElement("div");
    body.className = "story-recipe-body";
    var id = 0;
    groups.forEach(function (g) {
      if (g.title) { var gt = document.createElement("h4"); gt.textContent = g.title; body.appendChild(gt); }
      if (g.note) { var gn = document.createElement("p"); gn.className = "story-recipe-note"; gn.textContent = g.note; body.appendChild(gn); }
      var ul = document.createElement("ul");
      g.items.forEach(function (item) {
        var k = g.title + "|" + item;
        var li = document.createElement("li");
        var lab = document.createElement("label");
        var box = document.createElement("input");
        box.type = "checkbox";
        box.id = "story-ing-" + (id++);
        box.checked = !!ticked[k];
        box.addEventListener("change", function () {
          if (box.checked) ticked[k] = 1; else delete ticked[k];
          try { localStorage.setItem(key, JSON.stringify(ticked)); } catch (e) { /* private mode: this visit only */ }
        });
        lab.appendChild(box);
        lab.appendChild(document.createTextNode(" " + item));
        li.appendChild(lab);
        ul.appendChild(li);
      });
      body.appendChild(ul);
    });
    el.appendChild(head);
    if (steps.length) el.appendChild(stepEl);
    el.appendChild(body);

    function setOpen(open) {
      el.classList.toggle("is-collapsed", !open);
      toggle.textContent = open ? L.hide : L.show;
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    }
    // Open by default, everywhere, the show included; a reader who closes it
    // keeps it closed on the next visit (the step counter stays visible).
    var openKey = "ssg-story-recipe-open";
    toggle.addEventListener("click", function () {
      var open = el.classList.contains("is-collapsed");
      setOpen(open);
      try { localStorage.setItem(openKey, open ? "1" : "0"); } catch (e) { /* private mode */ }
    });
    var remembered = null;
    try { remembered = localStorage.getItem(openKey); } catch (e) { remembered = null; }
    // collapsed: true closes it everywhere, "phone" only on narrow screens.
    var startClosed = w.collapsed === true || (w.collapsed === "phone" && innerWidth < 700);
    setOpen(remembered === null ? !startClosed : remembered === "1");

    // A step's number is the one written in front of it ("5. …"), not its
    // position in the page; the total is the highest such number.
    function num(p) { return stepOf(p, stepRe); }
    var all = steps.reduce(function (m, p) { return Math.max(m, num(p)); }, 0);

    // The step the reader is on: the last one at or before where they are —
    // the narrator's paragraph, the slide in the presentation, or else the
    // reading line while scrolling.
    return function update() {
      var at = api.focus(), cur = null;
      steps.forEach(function (p) {
        var before = at ? (p === at || (p.compareDocumentPosition(at) & 4)) : api.top(p) < api.line();
        if (before) cur = p;
      });
      var n = cur ? num(cur) : 0;
      stepEl.textContent = n ? L.step.replace("{n}", n).replace("{all}", all) : "";
      stepEl.hidden = !n;
    };
  };
})();
