/*
 * ssg web-push service worker. push.js registers it from beside itself, so
 * its scope is the site (or the site's base path).
 *
 * A message is the JSON notify.ts sends: {"title", "body", "url", "icon"?}.
 * A click opens the post, or focuses a tab already showing it — but only for
 * a URL on this site; a payload pointing anywhere else opens the site root.
 */
self.addEventListener("push", function (event) {
  var data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch (e) {
    data = { title: event.data ? event.data.text() : "" };
  }
  var options = { body: data.body || "", data: { url: data.url || "" }, tag: data.url || undefined };
  if (data.icon) options.icon = data.icon;
  event.waitUntil(self.registration.showNotification(data.title || self.location.hostname, options));
});

self.addEventListener("notificationclick", function (event) {
  event.notification.close();
  var scope = self.registration.scope;
  var target = scope;
  try {
    var url = new URL(event.notification.data && event.notification.data.url, scope);
    if (url.origin === self.location.origin) target = url.href;
  } catch (e) {
    /* a malformed URL opens the site root */
  }
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (list) {
      for (var i = 0; i < list.length; i++) {
        if (list[i].url === target && "focus" in list[i]) return list[i].focus();
      }
      return self.clients.openWindow(target);
    }),
  );
});
