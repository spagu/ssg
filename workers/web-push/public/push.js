/*
 * ssg web push — the "notify me" button, progressive enhancement,
 * dependency-free.
 *
 * With `web_push: true` ssg puts this script in every page's <head>:
 *   <script src="/push.js" defer data-endpoint="/api/push"></script>
 * A theme only needs the button:
 *   <button type="button" data-push-subscribe hidden
 *           data-label-on="Notify me about new posts"
 *           data-label-off="Stop notifications">Notify me about new posts</button>
 *
 * The button stays hidden where the browser cannot do push (no service
 * worker, PushManager or Notification — on iOS that means the site is not
 * installed on the home screen). The permission prompt appears only after a
 * click, never on page load. A second click unsubscribes.
 *
 * sw.js is registered from beside this script, so a site under a base path
 * (/docs/push.js) gets /docs/sw.js with the scope /docs/.
 */
(function () {
  "use strict";

  var script = document.currentScript;
  var swURL = new URL("sw.js", (script && script.src) || location.href);
  var scope = new URL("./", swURL).pathname;
  var endpoint = ((script && script.getAttribute("data-endpoint")) || "/api/push").replace(/\/+$/, "");

  function supported() {
    return "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
  }

  function keyBytes(b64url) {
    var b64 = b64url.replace(/-/g, "+").replace(/_/g, "/");
    var bin = atob(b64 + "===".slice((b64.length + 3) % 4));
    var out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  function post(path, body) {
    return fetch(endpoint + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    }).then(function (res) {
      if (!res.ok) throw new Error("push " + path + ": " + res.status);
    });
  }

  function registration() {
    return navigator.serviceWorker.register(swURL.href, { scope: scope });
  }

  function subscribe(reg) {
    return fetch(endpoint + "/key")
      .then(function (res) {
        if (!res.ok) throw new Error("push key: " + res.status);
        return res.json();
      })
      .then(function (data) {
        return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(data.publicKey) });
      })
      .then(function (sub) {
        return post("/subscribe", sub.toJSON()).then(function () {
          return sub;
        });
      });
  }

  function unsubscribe(sub) {
    return post("/unsubscribe", { endpoint: sub.endpoint }).then(function () {
      return sub.unsubscribe();
    });
  }

  function show(buttons, on) {
    buttons.forEach(function (b) {
      var label = b.getAttribute(on ? "data-label-off" : "data-label-on");
      if (label) b.textContent = label;
      b.setAttribute("aria-pressed", on ? "true" : "false");
      b.hidden = false;
    });
  }

  function init() {
    var buttons = Array.prototype.slice.call(document.querySelectorAll("[data-push-subscribe]"));
    if (!buttons.length || !supported() || Notification.permission === "denied") return;

    // Reading the current state needs no prompt: getRegistration does not
    // register anything, so a visitor who never clicks leaves no worker behind.
    navigator.serviceWorker
      .getRegistration(scope)
      .then(function (reg) {
        return reg ? reg.pushManager.getSubscription() : null;
      })
      .then(function (sub) {
        show(buttons, !!sub);
      })
      .catch(function () {
        show(buttons, false);
      });

    buttons.forEach(function (b) {
      b.addEventListener("click", function () {
        buttons.forEach(function (x) {
          x.disabled = true;
        });
        registration()
          .then(function (reg) {
            return reg.pushManager.getSubscription().then(function (sub) {
              return sub ? unsubscribe(sub).then(function () { return false; })
                         : subscribe(reg).then(function () { return true; });
            });
          })
          .then(function (on) {
            show(buttons, on);
          })
          .catch(function (err) {
            if (window.console) console.warn(err);
            if (Notification.permission === "denied") buttons.forEach(function (x) { x.hidden = true; });
          })
          .then(function () {
            buttons.forEach(function (x) {
              x.disabled = false;
            });
          });
      });
    });
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
