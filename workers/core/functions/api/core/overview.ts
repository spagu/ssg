// Cloudflare Pages Function: every module this caller may see, with its
// summary, and what needs a person across all of them — the admin home page.
//   GET /api/core/overview[?lang=pl] →
//     {checkedAt, mode, user, lang,
//      items: [ModuleView & {summary: SummaryView | null, unreachableSince?: string | null}],
//      attention: {items: [{module, title, severity, text, href}], total}}
//
// summary: null when the module declares no summary; otherwise a state —
// ok | unavailable | denied | invalid (_summary.ts) — one module's trouble is
// never the whole answer's. core itself is not an item: the home page is its
// panel. attention: most serious first, then by module, at most ATTENTION_MAX
// (total says how many there were); each href is an absolute path on this
// host, joined from the module's base and refused (null) if it would leave it.
// Who sees what follows /api/core/modules (_visible.ts), and so do the module
// settings (GO-149): a hidden module's card and its attention items are left
// out, the rest come in the set order under their labels. unreachableSince,
// only with CORE_DB: when the module was last seen to stop answering, if it
// has not been seen answering since (_history.ts); null otherwise.

import { mergeAttention } from "./_attention";
import { mayManage } from "./_manage";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { gather } from "./_overview";
import { loadRegistry } from "./_registry";
import { route } from "./_route";
import { arrange, readState, relabel } from "./_settings";
import type { Caller } from "./_signin";
import { pickLang } from "./_view";

async function overview(request: Request, env: Env, caller: Caller): Promise<Response> {
  const loaded = await loadRegistry();
  if (!loaded.ok) return loaded.response;
  const lang = pickLang(request);
  const [all, state] = await Promise.all([gather(request, env, caller, lang, loaded.registry), readState(env)]);
  const gathered = arrange(all, (g) => g.entry.id, state?.settings)
    .filter((g) => g.entry.id !== "core")
    .map((g) => ({ ...g, view: relabel(g.view, state?.settings, lang) }));
  const attention = mergeAttention(
    gathered.map((g) => ({ id: g.entry.id, title: g.view.title, base: g.entry.base, summary: g.accepted })),
    lang,
  );
  const since = (id: string): { unreachableSince?: string | null } => {
    if (!state) return {};
    const last = state.latest.get(id);
    return { unreachableSince: last?.status === "unreachable" ? last.at : null };
  };
  const items = gathered.map((g) => ({ ...g.view, summary: g.summary, ...since(g.entry.id) }));
  return json({ checkedAt: new Date().toISOString(), mode: caller.mode, user: caller.user, lang, canManage: mayManage(caller), items, attention });
}

export const onRequest = route({ GET: overview });
