// The "?" list: every key the editor understands, from the same table the key
// handler and the button tooltips use (models/shortcuts.js).

import { h } from "../core/vnode.js";
import { HELP_ONLY, SHORTCUTS, isMac, keysOf } from "../models/shortcuts.js";
import { openDialog } from "../panel/dialog.js";

export async function showHelp(app) {
  const { t } = app;
  const mac = isMac(navigator.userAgentData?.platform ?? navigator.platform);
  const commands = [...new Set(SHORTCUTS.map(([c]) => c))];
  const row = (name, keys) => h("tr", null, h("th", { scope: "row" }, name), h("td", null, h("kbd", null, keys)));
  await openDialog({
    title: t("help.title"),
    wide: true,
    content: [
      h("div", { class: "table-scroll" }, h("table", { class: "data keys" },
        h("caption", null, t("help.caption")),
        h("thead", null, h("tr", null, h("th", { scope: "col" }, t("help.action")), h("th", { scope: "col" }, t("help.keys")))),
        h("tbody", null,
          commands.map((c) => row(t(`tb.${c}`), keysOf(c, mac))),
          HELP_ONLY.map(([k, keys]) => row(t(`help.${k}`), keys))))),
      h("p", { class: "hint" }, t("help.tabNote")),
    ],
    actions: [{ label: t("action.close"), value: null }],
  });
}
