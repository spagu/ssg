// Help (always in the same place in the navigation): what each part of the
// panel is for, where configuration lives, and how to read the charts by
// keyboard. Also the answer for an unknown address.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { pageHead } from "./ui.js";

const TOPICS = ["subscribers", "import", "campaigns", "sending", "suppressions", "settings", "charts", "privacy"];

export default async function help(app, root, route) {
  const { t } = app;
  const missing = route.name === "notFound";
  mount(root, [
    pageHead(app, missing ? t("help.notFound") : t("help.title"), { lead: missing ? t("help.notFound.lead") : t("help.lead") }),
    missing ? h("p", null, h("a", { class: "btn btn-primary", href: href("dashboard") }, t("nav.dashboard"))) : null,
    h(
      "div",
      { class: "help-grid" },
      TOPICS.map((k) => h("section", { class: "card", "aria-labelledby": `help-${k}` }, h("h2", { id: `help-${k}` }, t(`help.${k}.title`)), h("p", null, t(`help.${k}.body`)))),
    ),
  ]);
}
