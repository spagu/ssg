// The look preview's address (GO-188): api/preview with the page to show,
// the scheme, the panel's language and every colour and font token as the
// panel holds it now ("--name=value"). Pure; the worker checks it all again
// (src/preview-fetch.ts sitePath, src/preview-tokens.ts).

// isSitePath: what the worker accepts as a path — "/" then path characters,
// never "//" (another host) and no dot segment. A query may follow.
export function isSitePath(path) {
  if (typeof path !== "string" || path.length > 2048 || !/^\/[A-Za-z0-9\-._~!$&'()*+,;=:@%/?]*$/.test(path)) return false;
  const pathname = path.split("?", 1)[0];
  return !pathname.startsWith("//") && !/(^|\/)(\.|%2e){1,2}(\/|$)/i.test(pathname);
}

// TOKEN_VALUE: the values the worker applies (no url(), expression() or
// "//", src/preview-tokens.ts); others are left out here so a half-typed
// font stack does not turn the whole preview into an error page.
const TOKEN_VALUE = /^[A-Za-z0-9 #,.%()+\-/_'"]{1,200}$/;

// schemeValues: [name, value] for every token in `scheme` (dark falls back to
// light where the theme does not restate it), usable values only.
export function schemeValues(fields, scheme) {
  return fields
    .map((f) => [f.name, (scheme === "dark" ? f.dark ?? f.light : f.light ?? f.dark).replace(/\s+/g, " ").trim()])
    .filter(([, v]) => TOKEN_VALUE.test(v) && !/url\(|expression\(|\/\//i.test(v));
}

// previewHref: the frame's address, relative to the panel ("api/preview?…").
// page: "home", "latest" or "path" (with `path`).
export function previewHref({ page, path, scheme, lang, values }) {
  const q = new URLSearchParams();
  if (page === "latest") q.set("page", "latest");
  else q.set("path", page === "path" ? path : "/");
  q.set("scheme", scheme);
  q.set("lang", lang);
  for (const [name, value] of values) q.append(name, value);
  return `api/preview?${q}`;
}
