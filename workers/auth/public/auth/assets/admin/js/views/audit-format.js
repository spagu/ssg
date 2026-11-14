// One audit event in words — shared by the Audit log and a person's page.

import { h } from "../core/vnode.js";
import { detailPairs } from "../models/events.js";

export const kindLabel = (app, kind) => (app.has(`event.${kind}`) ? app.t(`event.${kind}`) : kind);

export function detailText(e) {
  return detailPairs(e.detail).map(([k, v]) => `${k}: ${v}`).join(" · ");
}

// eventLine: "<time> — <what> — by <who> — <detail>" for a compact list.
export function eventLine(app, e) {
  const { t, fmt } = app;
  return [
    h("time", { datetime: e.at }, fmt.dateTime(e.at)),
    " — ",
    h("strong", null, kindLabel(app, e.kind)),
    e.byEmail && e.by !== e.userId ? ` — ${t("audit.by", { email: e.byEmail })}` : "",
    detailText(e) ? h("span", { class: "sub" }, detailText(e)) : null,
  ];
}
