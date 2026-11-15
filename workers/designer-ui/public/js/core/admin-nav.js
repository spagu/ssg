// The site's shared admin bar (<ssg-admin-nav>, served by the `core` worker
// as /core/nav.js — GO-146) as this panel sees it. The bar is optional: on a
// site without `core` the element stays empty, zero pixels tall, and every
// function here answers "absent", so the panel lays itself out exactly as it
// did before the bar existed. No DOM is touched here; js/main.js applies the
// result.

export const ABSENT = Object.freeze({ height: 0, sticky: 0, signOut: false });

// readAdminNav measures the element. `style` is getComputedStyle (injected so
// a test runs without a browser). The bar counts as present only once it has
// drawn something (height > 0); it is sticky only if a page or a future
// nav.css makes it so — today it scrolls away with the page, so it never
// covers a focused control. `signOut`: the bar shows its own sign-out form
// (the `auth` mode), read from its open shadow root; if that ever stops being
// readable the answer is false and the panel keeps its own — duplicated, never
// lost.
export function readAdminNav(el, style) {
  const height = el ? Math.ceil(el.getBoundingClientRect().height) : 0;
  if (!height) return ABSENT;
  const position = style(el).position;
  return {
    height,
    sticky: position === "sticky" || position === "fixed" ? height : 0,
    signOut: Boolean(el.shadowRoot && el.shadowRoot.querySelector("form[action]")),
  };
}

// coverHeight is how much of the top of the viewport sticky things can cover,
// for scroll-padding-top (WCAG 2.2 2.4.11, focus not obscured): the panel's own
// sticky bar (`bar`, 0 on wide screens) stacked under the shared bar when that
// one is sticky, or a sticky notice whose bottom edge (`notice`) is lower.
export function coverHeight({ bar, notice, nav = ABSENT }) {
  return Math.ceil(Math.max(nav.sticky + bar, notice));
}

// navVars: the custom properties the layout reads (css/layout.css). With no
// bar they are removed (null), so the CSS falls back to exactly its old values.
export function navVars(nav) {
  return {
    "--admin-nav-height": nav.height ? `${nav.height}px` : null,
    "--admin-nav-sticky": nav.sticky ? `${nav.sticky}px` : null,
  };
}

// applyAdminNav writes the measurement onto the root element: the variables
// above (through the CSSOM, which the CSP allows) and the class
// `admin-nav-signout`, under which the panel hides its own "Signed in as /
// Sign out" because the bar already shows them. Absent: everything removed.
export function applyAdminNav(root, nav) {
  for (const [name, value] of Object.entries(navVars(nav))) {
    if (value) root.style.setProperty(name, value);
    else root.style.removeProperty(name);
  }
  root.classList.toggle("admin-nav-signout", nav.signOut);
}
