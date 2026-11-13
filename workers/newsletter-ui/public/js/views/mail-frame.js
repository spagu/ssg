// Shows a rendered mail inside the panel without weakening the panel's CSP.
//
// A srcdoc / about:blank frame inherits the page's Content-Security-Policy,
// and that policy (style-src 'self', no 'unsafe-inline') blocks the mail's
// <style> block and every style="" attribute — the preview would not look
// like the mail. Instead of relaxing the policy for the whole panel, the mail
// is parsed inert (DOMParser runs nothing), its CSS is lifted out, the nodes
// are moved into an empty frame, and the CSS is applied again through the
// CSSOM (a constructed stylesheet and element.style), which CSP does not
// govern. The frame is sandboxed without allow-scripts, so nothing in it can
// run, submit or navigate the panel; remote images stay blocked by img-src
// (a test send shows them).

export function mailFrame(doc, { html, title, className = "mail-frame" }) {
  const frame = doc.createElement("iframe");
  frame.className = className;
  frame.title = title;
  // allow-same-origin only, so the panel can write into it; no allow-scripts.
  frame.setAttribute("sandbox", "allow-same-origin");
  frame.addEventListener("load", () => fill(frame, html), { once: true });
  return frame;
}

function fill(frame, html) {
  const target = frame.contentDocument;
  if (!target) return;
  const parsed = new DOMParser().parseFromString(html, "text/html");
  const css = [];
  for (const s of parsed.querySelectorAll("style")) css.push(s.textContent), s.remove();
  for (const s of parsed.querySelectorAll("script, link, meta[http-equiv]")) s.remove();
  const inline = [];
  for (const el of parsed.querySelectorAll("[style]")) inline.push([el, el.getAttribute("style")]), el.removeAttribute("style");
  const root = target.adoptNode(parsed.documentElement);
  target.replaceChild(root, target.documentElement);
  for (const [el, text] of inline) el.style.cssText = text;
  try {
    const sheet = new frame.contentWindow.CSSStyleSheet();
    sheet.replaceSync(css.join("\n"));
    target.adoptedStyleSheets = [sheet];
  } catch {
    // An old browser without constructed stylesheets: the inline styles
    // (the mail's real layout) still apply; only the extras are missing.
  }
}
