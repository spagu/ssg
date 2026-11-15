// A setting's value shown without a control — for an account that may read
// but not change it, and in the revision view. Data goes in as text only.
// A secret is only ever "set" / "not set" and its reference.

import { h } from "../core/vnode.js";
import { isRedacted } from "../models/secret-ref.js";

const MAX = 20;

export function valueView(app, v, depth = 0) {
  const { t } = app;
  if (v === undefined || v === null) return h("span", { class: "muted" }, t("value.unset"));
  if (isRedacted(v)) return h("span", null, h("span", { class: `chip ${v.set ? "chip-set" : "chip-unset"}` }, t(v.set ? "secret.set" : "secret.unset")), v.reference ? [" ", h("code", null, v.reference)] : null);
  if (typeof v === "boolean") return h("span", null, t(v ? "value.on" : "value.off"));
  if (typeof v === "string") return v === "" ? h("span", { class: "muted" }, t("value.empty")) : h("code", { class: "value-text" }, v);
  if (typeof v === "number") return h("code", null, String(v));
  if (depth > 2) return h("code", null, JSON.stringify(v));
  if (Array.isArray(v)) {
    if (!v.length) return h("span", { class: "muted" }, t("value.items", { count: 0 }));
    return h("ul", { class: "value-list" }, v.slice(0, MAX).map((e) => h("li", null, valueView(app, e, depth + 1))), v.length > MAX ? h("li", { class: "muted" }, t("value.more", { count: v.length - MAX })) : null);
  }
  const entries = Object.entries(v);
  if (!entries.length) return h("span", { class: "muted" }, t("value.keys", { count: 0 }));
  return h("dl", { class: "value-map" }, entries.slice(0, MAX).map(([k, e]) => [h("dt", null, k), h("dd", null, valueView(app, e, depth + 1))]));
}
