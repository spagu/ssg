// GO-146: the panel's side of the site's shared admin bar — measuring it,
// the sticky offsets with and without it, and what is written to the page.
// Absent (no `core`), every answer must leave the layout as it always was.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ABSENT, applyAdminNav, coverHeight, navVars, readAdminNav } from "../public/js/core/admin-nav.js";

const navEl = (height: number, signOut: boolean | null = false) => ({
  getBoundingClientRect: () => ({ height }),
  shadowRoot: signOut === null ? null : { querySelector: (sel: string) => (signOut && sel === "form[action]" ? {} : null) },
});
const style = (position: string) => () => ({ position });

test("readAdminNav: no element, or an element that drew nothing, is absent", () => {
  assert.equal(readAdminNav(null, style("static")), ABSENT);
  assert.equal(readAdminNav(navEl(0), style("static")), ABSENT);
});

test("readAdminNav: a drawn bar — height rounded up, sticky only when positioned so", () => {
  assert.deepEqual(readAdminNav(navEl(51.2), style("static")), { height: 52, sticky: 0, signOut: false });
  assert.deepEqual(readAdminNav(navEl(52), style("sticky")), { height: 52, sticky: 52, signOut: false });
  assert.deepEqual(readAdminNav(navEl(52), style("fixed")), { height: 52, sticky: 52, signOut: false });
});

test("readAdminNav: the bar's own sign-out is seen; an unreadable shadow root means none", () => {
  assert.equal(readAdminNav(navEl(52, true), style("static")).signOut, true);
  assert.equal(readAdminNav(navEl(52, null), style("static")).signOut, false);
});

test("coverHeight: without the bar exactly the old rule, max(bar, notice) rounded up", () => {
  assert.equal(coverHeight({ bar: 0, notice: 0 }), 0);
  assert.equal(coverHeight({ bar: 61.4, notice: 0 }), 62);
  assert.equal(coverHeight({ bar: 61, notice: 130.2, nav: ABSENT }), 131);
});

test("coverHeight: a scrolling bar covers nothing; a sticky one stacks above the panel's bar", () => {
  const scrolling = { height: 52, sticky: 0, signOut: false };
  const sticky = { height: 52, sticky: 52, signOut: false };
  assert.equal(coverHeight({ bar: 61, notice: 0, nav: scrolling }), 61);
  assert.equal(coverHeight({ bar: 61, notice: 0, nav: sticky }), 113);
  assert.equal(coverHeight({ bar: 0, notice: 0, nav: sticky }), 52);
  // The notice's measured top already includes the bar's offset (css/layout.css).
  assert.equal(coverHeight({ bar: 61, notice: 200, nav: sticky }), 200);
});

test("navVars: absent removes both variables; present sets what applies", () => {
  assert.deepEqual(navVars(ABSENT), { "--admin-nav-height": null, "--admin-nav-sticky": null });
  assert.deepEqual(navVars({ height: 52, sticky: 0, signOut: false }), { "--admin-nav-height": "52px", "--admin-nav-sticky": null });
  assert.deepEqual(navVars({ height: 52, sticky: 52, signOut: true }), { "--admin-nav-height": "52px", "--admin-nav-sticky": "52px" });
});

test("applyAdminNav: writes the variables and the sign-out class, and takes them back", () => {
  const props = new Map<string, string>();
  const classes = new Set<string>();
  const root = {
    style: { setProperty: (k: string, v: string) => props.set(k, v), removeProperty: (k: string) => props.delete(k) },
    classList: { toggle: (c: string, on: boolean) => (on ? classes.add(c) : classes.delete(c)) },
  };
  applyAdminNav(root, { height: 52, sticky: 52, signOut: true });
  assert.deepEqual(Object.fromEntries(props), { "--admin-nav-height": "52px", "--admin-nav-sticky": "52px" });
  assert.deepEqual([...classes], ["admin-nav-signout"]);
  applyAdminNav(root, ABSENT);
  assert.equal(props.size, 0);
  assert.equal(classes.size, 0);
});
