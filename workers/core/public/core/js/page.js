// What the page asks for, in which order, and the whole tree it draws from
// the answers. No DOM: main.js mounts what this returns.
//
// ASKED ONLY ON LOAD AND ON "REFRESH" — NEVER ON A TIMER. Every summary is a
// few D1 statements counting whole tables in each module (rows read are
// billed), so a home page left open must not keep asking. core also keeps
// each answer for CORE_STATUS_CACHE_SECONDS per caller.
//
// Order: the overview (summaries) and the status (health checks) together;
// then the info, which reads the same summaries — by then from core's cache,
// so the modules are not asked twice.

import { h } from "./vnode.js";
import { statusIndex, unanswered } from "./model.js";
import { attentionSection } from "./views/attention.js";
import { modulesSection } from "./views/modules.js";
import { errorBlock, loading, pageHead } from "./views/shell.js";
import { siteSection } from "./views/site.js";
import { appearanceView } from "./views/appearance.js";
import { manageView } from "./views/manage.js";

// loadAll returns {overview, status, info, error}: the overview failing is
// the page failing (error set); a failed status or info only leaves its part
// out (null) — the rest still shows.
export async function loadAll(api, lang) {
  const [overview, status] = await Promise.all([api.get("overview", { lang }).catch((e) => ({ failed: e })), api.get("status").catch(() => null)]);
  if (overview.failed) return { overview: null, status: null, info: null, error: overview.failed };
  const info = await api.get("info", { lang }).catch((e) => ({ failed: e }));
  return { overview, status, info, error: null };
}

// pageTree: the heading and, below it, loading / the error / the three sections.
export function pageTree(app, data, busy) {
  if (app.route === "appearance") return appearanceTree(app, data);
  // The Modules view (GO-149) loads its own answer (app.manage, manage-state.js).
  if (app.route === "modules") return manageView(app);
  const head = pageHead(app, { busy });
  if (!data) return [head, loading(app)];
  if (data.error) return [head, errorBlock(app, data.error, () => app.refresh())];
  const items = data.overview.items;
  const titles = new Map(items.map((i) => [i.id, i.title]));
  const scoped = { ...app, titleOf: (id) => titles.get(id) ?? id };
  return [
    head,
    attentionSection(scoped, data.overview.attention, unanswered(items)),
    modulesSection(scoped, items, statusIndex(data.status), data.overview.canManage === true),
    data.info.failed
      ? h("section", { class: "site", "aria-labelledby": "site-title" }, h("h2", { id: "site-title" }, app.t("site.title")), errorBlock(app, data.info.failed, () => app.refresh()))
      : siteSection(scoped, data.info, items),
  ];
}

// appearanceTree: /core/#appearance — the theme comes with /api/core/info,
// so the view waits for the same answers as the home page.
function appearanceTree(app, data) {
  if (!data) return [loading(app)];
  const failed = data.error ?? data.info?.failed;
  if (failed) return [errorBlock(app, failed, () => app.refresh())];
  return appearanceView(app, data.info.theme ?? null);
}
