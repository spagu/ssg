// What Appearance and Files show when the theme cannot be edited here: a
// theme built into ssg or downloaded at build time is not in the repository,
// so there is nothing to commit to. The way out is said in words: export the
// theme into the repository once (ssg writes its files), commit, reload.
// Settings keep working either way.

import { h } from "../core/vnode.js";
import { emptyState } from "../panel/ui.js";

export function themeUnavailable(app, theme) {
  const { t } = app;
  const reason = ["theme_not_in_repository", "online_theme", "no_template", "theme_dir_unknown"].includes(theme.reason) ? theme.reason : "other";
  const vars = { template: theme.template ?? "", dir: theme.dir ?? "templates" };
  return [
    emptyState({ img: "theme-export", title: t(`theme.${reason}.title`, vars), body: t(`theme.${reason}.body`, vars) }),
    reason === "theme_not_in_repository"
      ? h(
          "div",
          { class: "card" },
          h("h2", null, t("theme.export.title")),
          h(
            "ol",
            { class: "plain steps" },
            h("li", null, t("theme.export.step1"), " ", h("code", null, "ssg --config .ssg.yaml")),
            h("li", null, t("theme.export.step2", vars), " ", h("code", null, `git add ${vars.dir}/`)),
            h("li", null, t("theme.export.step3")),
          ),
        )
      : null,
    theme.detail ? h("p", { class: "hint" }, t("theme.detail", { detail: theme.detail })) : null,
  ];
}
